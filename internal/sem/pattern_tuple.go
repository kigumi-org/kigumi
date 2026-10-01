package sem

import "kigumi/internal/syntax"

// tuplePattern handles `(a, b)` over a TupleN scrutinee: the same
// shape as recordPattern's structural branch, but positional and without a
// path to resolve, since a tuple's fields are named by position alone.
func (c *checker) tuplePattern(p syntax.NodeID, scrutinee TypeID, seen map[string]syntax.NodeID) PatInfo {
	tt := c.r.Types
	subs := c.t.Children(p)
	ent, ok := tt.tupleEntity(len(subs))
	if !ok {
		c.errAt(p, cTupleArity, len(subs))
		for _, s := range subs {
			c.patternInner(s, TyPoison, false, seen)
		}
		c.setType(p, TyPoison)
		return PatInfo{Kind: PatWild, Type: TyPoison}
	}
	sn := tt.Node(scrutinee)
	if scrutinee != TyPoison && (sn.Kind != KNamed || sn.Ent != ent) {
		c.errAt(p, cPatternKind, scrutinee)
		for _, s := range subs {
			c.patternInner(s, TyPoison, false, seen)
		}
		c.setType(p, TyPoison)
		return PatInfo{Kind: PatWild, Type: scrutinee}
	}
	c.info.Uses[p] = ent
	moves := false
	for i, s := range subs {
		ft := TyPoison
		if scrutinee != TyPoison {
			ft = sn.Args[i]
		}
		if c.patternInner(s, ft, false, seen).Moves {
			moves = true
		}
	}
	return PatInfo{Kind: PatRecordTy, Type: scrutinee, Moves: moves}
}
