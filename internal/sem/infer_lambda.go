package sem

import "kigumi/internal/syntax"

// lambdaCtx: one lambda body under check.
type lambdaCtx struct {
	ent      EntityID
	scope    ScopeID
	captured map[EntityID]int
	list     []Capture
	// refCapture: an E412 (borrow captured) already explains a borrow-typed body.
	refCapture bool
	immediate  bool
}

// synthLambda: want is a fn type, an Fn-constrained type parameter, or 0
// (then every parameter needs an annotation).
func (c *checker) synthLambda(n syntax.NodeID, want TypeID) TypeID {
	node := c.t.Nodes[n]
	params := c.t.Children(syntax.NodeID(node.Lhs))
	body := syntax.NodeID(node.Rhs)
	tt := c.r.Types
	target := c.lambdaTarget(want)
	if target != 0 && len(tt.Node(target).Args) != len(params) {
		c.errAt(n, cLambdaArity, len(params), plural(len(params)), target, len(tt.Node(target).Args))
		return TyPoison
	}
	id := c.r.newEntity(Entity{Kind: EntClosure, Name: "closure", Pkg: c.pkg, File: c.f, Node: n, Tok: node.Tok, Parent: c.fn})
	c.r.Entities[id].Detail = c.r.addClosure(ClosureInfo{})
	c.info.Defs[n] = id
	saved := c.scope
	c.pushScope(ScopeLambda, n)
	ctx := &lambdaCtx{ent: id, scope: c.scope, captured: map[EntityID]int{}, immediate: c.argLambda[n]}
	c.lambdas = append(c.lambdas, ctx)
	ptypes, pents, bad := c.lambdaParams(id, params, target)
	ret := TypeID(0)
	if target != 0 {
		ret = tt.Node(target).Elem
	}
	savedEdges := c.edges
	var edges []EffectEdge
	c.edges = &edges
	ret = c.lambdaBody(body, ret)
	c.edges = savedEdges
	c.lambdas = c.lambdas[:len(c.lambdas)-1]
	c.popScope(saved)
	if bad {
		return TyPoison
	}
	info := c.r.closure(id)
	info.Params = pents
	info.Ret = ret
	info.Sig = tt.Fn(ptypes, ret, 0, false)
	info.Captures = ctx.list
	info.Copy = closureCopy(ctx.list)
	info.Immediate = ctx.immediate
	info.Edges = edges
	c.closures = append(c.closures, id)
	return tt.Closure(id)
}

func (c *checker) lambdaTarget(want TypeID) TypeID {
	want = c.vars.resolve(want)
	if want == 0 {
		return 0
	}
	tt := c.r.Types
	switch tt.Kind(want) {
	case KFn:
		return want
	case KVar:
		return c.vars.fnHint(want)
	case KParam:
		for _, con := range c.r.typeParam(tt.Node(want).Ent).Constraints {
			if con.Kind == CFn || con.Kind == CFnMut || con.Kind == CFnOnce {
				return con.Type
			}
		}
	}
	return 0
}

func (c *checker) lambdaParams(closure EntityID, params []syntax.NodeID, target TypeID) ([]TypeID, []EntityID, bool) {
	tt := c.r.Types
	var types []TypeID
	var ents []EntityID
	bad := false
	for i, p := range params {
		ps := param(c.t, p)
		name := c.t.TokText(c.t.Nodes[p].Tok)
		var pt TypeID
		if target != 0 {
			pt = c.vars.settleLiterals(tt.Node(target).Args[i])
		}
		switch {
		case ps.Type != 0:
			annotated := c.r.resolveType(c.f, c.scope, ps.Type, posLocal)
			if pt != 0 && annotated != pt && !c.vars.unify(annotated, pt) {
				c.errAt(p, cLambdaParamMismatch, name, annotated, pt)
				bad = true
			}
			pt = annotated
		case pt == 0 || tt.ContainsVar(pt):
			c.errAt(p, cLambdaParamAnnot, name)
			pt = TyPoison
			bad = true
		}
		pe := Entity{Kind: EntParam, Name: name, Pkg: c.pkg, File: c.f, Node: p, Tok: c.t.Nodes[p].Tok, Parent: closure, Type: pt}
		if ps.Flags&syntax.FlagMut != 0 {
			pe.Flags |= EfMut
		}
		pid := c.r.newEntity(pe)
		c.r.Entities[pid].Detail = c.r.addLocal(LocalInfo{Scope: c.scope})
		c.info.Defs[p] = pid
		if _, dup := c.r.Scopes[c.scope].Names[name]; dup {
			c.errAt(p, cParamDuplicate, name)
		} else if name != "_" {
			c.r.Scopes[c.scope].Names[name] = Binding{Ent: pid}
		}
		c.setType(p, pt)
		types = append(types, pt)
		ents = append(ents, pid)
	}
	return types, ents, bad
}

func (c *checker) lambdaBody(body syntax.NodeID, ret TypeID) TypeID {
	savedRet, savedLoop, savedCleanup, savedStmt := c.retType, c.loop, c.cleanup, c.stmtPos
	savedBreaks := c.breaks
	savedFirst := c.vars.first
	entryFacts := c.snapshotFacts()
	c.loop, c.cleanup, c.breaks = 0, 0, nil
	tt := c.r.Types
	if ret == 0 {
		ret = c.vars.fresh(body, "return")
	}
	c.retType = ret
	c.vars.first = c.vars.count()
	if !tt.ContainsVar(c.vars.resolve(ret)) {
		c.check(body, ret)
	} else {
		got := c.vars.resolve(c.synth(body))
		if tt.Kind(got) == KUntyped {
			got = c.adopt(c.literalNode(body), defaultOf(got))
		}
		if got == TyPoison {
			c.vars.poison(ret)
		} else if got != TyNever && !c.vars.unify(ret, got) {
			c.mismatch(body, ret, got)
		}
	}
	c.retType, c.loop, c.cleanup, c.stmtPos = savedRet, savedLoop, savedCleanup, savedStmt
	c.breaks = savedBreaks
	c.vars.first = savedFirst
	c.facts = entryFacts
	out := c.vars.resolve(ret)
	if tt.Kind(out) == KRef && !c.lambdas[len(c.lambdas)-1].refCapture {
		c.errAt(body, cBorrowPosition, "return types")
		return TyPoison
	}
	return out
}

func (c *checker) lambdaRetUnknown() bool {
	return len(c.lambdas) > 0 && c.r.Types.Kind(c.vars.resolve(c.retType)) == KVar
}
