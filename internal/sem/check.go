package sem

import (
	"kigumi/internal/diag"
	"kigumi/internal/syntax"
)

type checker struct {
	r          *Result
	f          FileID
	t          *syntax.Tree
	info       *FileInfo
	pkg        PackageID
	fn         EntityID
	retType    TypeID
	scope      ScopeID
	loop       int
	brokeOut   bool
	breaks     *loopBreaks
	allocDepth int
	stmtPos    syntax.NodeID
	// wantAliasTarget/File/Node name the declared type-annotation node
	// for a mismatch reported on wantAliasTarget; the target check
	// keeps a stale hint from a still-open outer check() leaking into an
	// unrelated mismatch that a nested check() reports first.
	wantAliasTarget syntax.NodeID
	wantAliasFile   FileID
	wantAliasNode   syntax.NodeID
	cleanup         int
	edges           *[]EffectEdge
	vars            *varStore
	touched         []syntax.NodeID
	// piped maps a call node to the `|>` left operand it takes first.
	piped map[syntax.NodeID]syntax.NodeID
	// argLambda marks a lambda written directly as a call argument; it may capture borrows, the callee not expected to keep them.
	argLambda map[syntax.NodeID]bool
	// patBorrow is set while checking a pattern whose scrutinee is borrowed:
	// bindings then borrow their payloads instead of moving them.
	patBorrow bool
	// patBorrowMut is patBorrow's mutability, meaningful only when patBorrow
	// is set (a `for` head can borrow mutably, unlike match).
	patBorrowMut bool
	contracts    []Effects
	lambdas      []*lambdaCtx
	// captureSites indexes every Capture entry recorded for a local, across
	// closures open and closed, so a write seen after a closure finishes can
	// still promote that closure's entry.
	captureSites map[EntityID][]captureSite
	facts        []NarrowFact
	inCond       int
	unsafe       int
	naked        bool
	closures     []EntityID
	constraints  []constraintCheck
	witnessTasks []witnessTask
	unsatIface   map[unsatKey]bool
	// paramAlias maps an immutable local bound to a fn-typed parameter
	// (`let g = f`) back to that parameter for callback polymorphism.
	paramAlias map[EntityID]EntityID
	// fieldFact stashes a resolved `pure?` field-call's FactPureField
	// between callMethod finding it and callValue adding
	// the matching effect edge.
	fieldFact map[syntax.NodeID]NarrowFact
}

// checkBodies (pass 7) type-checks every function, test and implicit main
// in entity order, which is package, file and declaration order.
func (r *Result) checkBodies() {
	for _, id := range r.implicitMain {
		r.checkImplicitMain(id)
	}
	for id := 1; id < len(r.Entities); id++ {
		e := &r.Entities[id]
		if e.File == 0 || e.Flags&EfPoison != 0 {
			continue
		}
		switch e.Kind {
		case EntFn:
			info := r.Fn(EntityID(id))
			if info.Body != 0 && r.Entities[e.Parent].Kind != EntInterface {
				r.checkFnBody(EntityID(id))
			}
		case EntTest:
			r.checkTestBody(EntityID(id))
		}
	}
	r.bodiesChecked = true
}

func (r *Result) newChecker(f FileID, fn EntityID, scope ScopeID, ret TypeID, edges *[]EffectEdge) *checker {
	return &checker{r: r, f: f, t: r.tree(f), info: &r.Files[f], pkg: r.packageOf(f), fn: fn, retType: ret, paramAlias: map[EntityID]EntityID{},
		captureSites: map[EntityID][]captureSite{}, scope: scope, edges: edges, vars: newVarStore(r)}
}

func (r *Result) checkFnBody(id EntityID) {
	e := &r.Entities[id]
	info := r.Fn(id)
	var edges []EffectEdge
	c := r.newChecker(e.File, id, r.declScope(id), r.Types.Node(info.Sig).Elem, &edges)
	r.Bodies = append(r.Bodies, BodyRef{Fn: id, File: e.File, Node: info.Body})
	if info.Naked {
		c.naked = true
		if stmts := c.t.Children(info.Body); len(stmts) != 1 || c.t.Kind(stmts[0]) != syntax.ExprStmt || c.t.Kind(syntax.NodeID(c.t.Nodes[stmts[0]].Lhs)) != syntax.AsmExpr {
			r.errAt(e.File, info.Body, cNakedBody)
		}
	}
	c.checkBodyBlock(info.Body)
	r.Fn(id).Edges = edges
}

// checkBodyBlock checks a function body against the return type: the block's
// tail value must coerce to it, and a missing tail needs Unit or Never.
func (c *checker) checkBodyBlock(body syntax.NodeID) {
	got := c.check(body, c.retType)
	_ = got
	c.finish()
}

func (c *checker) pushScope(kind ScopeKind, node syntax.NodeID) ScopeID {
	s := c.r.newScope(kind, c.scope, c.f, node)
	c.r.Scopes[s].Fn = c.fn
	c.info.Scopes[node] = s
	c.scope = s
	return s
}

func (c *checker) popScope(saved ScopeID) { c.scope = saved }

func (c *checker) errAt(n syntax.NodeID, code Code, args ...any) bool {
	return c.r.errAt(c.f, n, code, args...)
}

func (c *checker) errFix(n syntax.NodeID, fix diag.Fix, code Code, args ...any) bool {
	return c.r.errFix(c.f, n, fix, code, args...)
}

func (c *checker) loc(n syntax.NodeID) diag.Location { return diag.At(c.t.File, c.t.Span(n)) }

func (c *checker) replaceNode(n syntax.NodeID, title, text string) diag.Fix {
	return diag.Fix{Title: title, Loc: c.loc(n), NewText: text}
}

func (c *checker) setType(n syntax.NodeID, t TypeID) TypeID {
	c.info.Types[n] = t
	c.touched = append(c.touched, n)
	return t
}

// finish closes a full expression: unresolved inference variables
// default or become errors, and every type recorded since the last boundary
// is substituted to its final form.
func (c *checker) finish() {
	c.vars.settle(c)
	c.checkConstraints()
	for _, id := range c.closures {
		info := c.r.closure(id)
		info.Ret = c.vars.resolve(info.Ret)
		info.Sig = c.vars.resolve(info.Sig)
	}
	c.closures = c.closures[:0]
	for _, n := range c.touched {
		c.info.Types[n] = c.vars.resolve(c.info.Types[n])
		// Bindings keep the resolved type too; the backends read it from
		// the entity, not from the node.
		if def := c.info.Defs[n]; def != 0 {
			e := &c.r.Entities[def]
			e.Type = c.vars.resolve(e.Type)
			if c.r.Types.Kind(e.Type) == KUntyped {
				e.Type = defaultOf(e.Type)
			}
		}
		if co, ok := c.info.Coerce[n]; ok {
			co.From = c.vars.resolve(co.From)
			for i := range co.Steps {
				co.Steps[i].To = c.vars.resolve(co.Steps[i].To)
			}
			c.info.Coerce[n] = co
		}
		if call, ok := c.info.Calls[n]; ok && len(call.Inst) > 0 {
			for i := range call.Inst {
				call.Inst[i] = c.vars.resolve(call.Inst[i])
			}
			c.info.Calls[n] = call
		}
	}
	c.touched = c.touched[:0]
	c.vars.boundary()
}
