package sem

import "kigumi/internal/syntax"

// rangePattern types `lo..hi` / `lo..=hi`: both bounds
// are integer literals or named constants, adopted to the scrutinee's
// integer type. Ranges never make an integer match exhaustive.
func (c *checker) rangePattern(p syntax.NodeID, scrutinee TypeID) PatInfo {
	node := c.t.Nodes[p]
	lo, hi := syntax.NodeID(node.Lhs), syntax.NodeID(node.Rhs)
	tt := c.r.Types
	if scrutinee != TyPoison && !tt.IsInteger(scrutinee) {
		c.errAt(p, cRangePatternType, scrutinee)
		c.setType(p, TyPoison)
		scrutinee = TyPoison
	}
	okLo := c.rangeBoundValue(lo, scrutinee)
	okHi := c.rangeBoundValue(hi, scrutinee)
	if !okLo || !okHi {
		c.setType(p, TyPoison)
		return PatInfo{Kind: PatWild, Type: TyPoison}
	}
	if c.info.Literals[c.literalNode(lo)].Int.Cmp(c.info.Literals[c.literalNode(hi)].Int) > 0 {
		c.errAt(p, cRangePatternOrder)
	}
	return PatInfo{Kind: PatRange, Type: scrutinee}
}

// The grammar admits only an integer literal or bare identifier, so the one
// extra check beyond the literal-pattern rule is that an identifier resolves
// to a constant, not a binding it would otherwise shadow.
func (c *checker) rangeBoundValue(n syntax.NodeID, scrutinee TypeID) bool {
	got := c.vars.resolve(c.synth(n))
	if c.t.Kind(n) == syntax.Ident && got != TyPoison {
		ent := c.info.Uses[n]
		if ent == 0 || c.r.Entities[ent].Kind != EntConst {
			c.errAt(n, cRangeBoundNotConst)
			c.setType(n, TyPoison)
			return false
		}
	}
	tt := c.r.Types
	switch {
	case scrutinee == TyPoison || got == TyPoison:
		return false
	case tt.Kind(got) == KUntyped:
		if !tt.IsInteger(scrutinee) {
			return false
		}
		c.adopt(c.literalNode(n), scrutinee)
	case got != scrutinee:
		c.errAt(n, cPatternLiteralType, c.t.TokText(c.t.Nodes[c.literalNode(n)].Tok), scrutinee)
		c.setType(n, TyPoison)
		return false
	}
	return c.info.Types[n] != TyPoison
}
