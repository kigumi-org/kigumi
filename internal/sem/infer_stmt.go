package sem

import (
	"kigumi/internal/diag"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// stmt checks one statement and returns its type.
func (c *checker) stmt(n syntax.NodeID) TypeID {
	node := c.t.Nodes[n]
	var t TypeID
	switch node.Kind {
	case syntax.LetStmt:
		c.letStmt(n)
		t = TyUnit
	case syntax.AssignStmt:
		c.assignStmt(n)
		t = TyUnit
	case syntax.ExprStmt:
		t = c.exprStmt(n)
	case syntax.DeferStmt, syntax.ErrdeferStmt:
		c.cleanupStmt(n)
		t = TyUnit
	default:
		t = c.synth(n)
	}
	c.finish()
	c.setType(n, t)
	return t
}

func (c *checker) exprStmt(n syntax.NodeID) TypeID {
	e := syntax.NodeID(c.t.Nodes[n].Lhs)
	c.stmtPos = e
	t := c.vars.resolve(c.synth(e))
	if c.r.Types.Kind(t) == KUntyped {
		t = c.adopt(c.literalNode(e), defaultOf(t))
	}
	if _, _, ok := c.r.Types.IsResult(t); ok {
		start := c.t.Span(e).Start
		c.errFix(e, diag.Fix{Title: "Discard with `_ =`", Loc: diag.At(c.t.File, token.Span{Start: start, End: start}), NewText: "_ = "}, cUnusedResult, t)
		return TyPoison
	}
	if t == TyNever {
		return TyNever
	}
	if t != TyUnit && t != TyPoison {
		c.edge(EffectEdge{Kind: EdgeDrop, Type: t, Node: e})
		c.errAt(e, cUnusedValue, t)
	}
	return t
}

func (c *checker) letStmt(n syntax.NodeID) {
	s := letStmt(c.t, n)
	var annotated TypeID
	if s.Type != 0 {
		annotated = c.r.resolveType(c.f, c.scope, s.Type, posLocal)
	}
	var got TypeID
	if annotated != 0 {
		got = c.checkAlias(s.Init, annotated, c.f, s.Type)
	} else {
		got = c.synth(s.Init)
		if c.r.Types.Kind(c.vars.resolve(got)) == KUntyped {
			got = c.adopt(c.literalNode(s.Init), defaultOf(c.vars.resolve(got)))
		}
	}
	if s.Else != 0 {
		c.letElse(n, s, got)
		return
	}
	mut := s.Flags&syntax.FlagMut != 0
	if c.t.Kind(s.Pattern) == syntax.PatBind {
		typ := got
		borrow := false
		switch {
		case annotated != 0:
			typ = annotated
		case c.borrowedPlace(s.Init) && !c.r.isCopy(c.vars.resolve(got)) && c.r.Types.Kind(c.vars.resolve(got)) != KRef:
			typ, borrow = c.r.Types.Ref(c.vars.resolve(got), false), true
		}
		c.info.Pats[s.Pattern] = PatInfo{Kind: PatBinding, Type: got, Borrow: borrow}
		c.declareLocal(s.Pattern, c.t.TokText(c.t.Nodes[s.Pattern].Tok), typ, mut)
		c.setType(s.Pattern, typ)
		// `let g = f` keeps a callback parameter's polymorphism: calls of g
		// count as calls of f.
		if p := c.polyParam(s.Init); p != 0 && !mut {
			c.paramAlias[c.info.Defs[s.Pattern]] = p
		}
		if c.t.Kind(s.Init) == syntax.RecordLit {
			c.recordEffectFacts(s.Init, c.r.identPlace(c.info.Defs[s.Pattern]))
		}
		return
	}
	pi := c.checkPatternOn(s.Pattern, s.Init, got, mut, c.scope)
	if !c.irrefutable(s.Pattern, got) {
		c.errAt(s.Pattern, cPatternRefutable, c.witnessText(s.Pattern, got))
	}
	_ = pi
}

// letElse handles `let pat = e else { ... }`: bindings live after the
// statement and the else block must diverge (CTL-4).
func (c *checker) letElse(n syntax.NodeID, s letSlots, got TypeID) {
	saved := c.scope
	elseType := c.check(s.Else, 0)
	if c.vars.resolve(elseType) != TyNever {
		c.errAt(s.Else, cLetElseMustDiverge)
	}
	c.scope = saved
	c.checkPatternOn(s.Pattern, s.Init, got, s.Flags&syntax.FlagMut != 0, c.scope)
	if c.irrefutable(s.Pattern, got) {
		c.errAt(s.Pattern, cLetElseIrrefutable)
	}
	pos, _ := c.patternFacts(s.Pattern, c.factPlace(s.Init))
	c.addFacts(pos)
	c.info.Matches[n] = MatchInfo{Exhaustive: false}
}

func (c *checker) assignStmt(n syntax.NodeID) {
	node := c.t.Nodes[n]
	lhs, rhs := syntax.NodeID(node.Lhs), syntax.NodeID(node.Rhs)
	op := c.t.Toks[node.Tok].Kind
	if c.t.Kind(lhs) == syntax.Ident && c.t.TokText(c.t.Nodes[lhs].Tok) == "_" && !op.IsCompoundAssign() {
		t := c.synth(rhs)
		c.edge(EffectEdge{Kind: EdgeDrop, Type: t, Node: rhs})
		return
	}
	target := c.synth(lhs)
	if c.indexAssignUnsupported(lhs) {
		c.errAt(lhs, cIndexAssignUnsupported)
	} else if !c.isMutablePlace(lhs) {
		c.reportImmutable(lhs)
	} else if root := c.rootOf(lhs); root != 0 {
		c.noteCaptureWrite(lhs, root)
	}
	c.invalidatePlace(lhs)
	if op.IsCompoundAssign() {
		c.compoundAssign(n, lhs, rhs, target)
		return
	}
	if elem, ok := c.r.ScalarRefElem(target); ok && c.isIdentPlace(lhs) {
		target = elem
	}
	c.check(rhs, target)
	if c.t.Kind(rhs) == syntax.RecordLit {
		if place := c.placeOf(lhs); place != 0 {
			c.recordEffectFacts(rhs, c.r.Places[place])
		}
	}
}

// cleanupStmt checks a defer/errdefer body as a Unit statement with
// control flow forbidden inside.
func (c *checker) cleanupStmt(n syntax.NodeID) {
	node := c.t.Nodes[n]
	if node.Kind == syntax.ErrdeferStmt && !c.isResultFn() {
		c.errAt(n, cErrdeferOutsideResult)
	}
	c.cleanup++
	body := syntax.NodeID(node.Lhs)
	if c.t.Kind(body) == syntax.Block {
		c.check(body, TyUnit)
	} else {
		c.stmt(body)
	}
	c.cleanup--
}
