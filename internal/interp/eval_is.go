package interp

import "kigumi/internal/syntax"

// isExpr evaluates `v is pattern`. A temporary nothing binds is dead after
// the test; bindings keep what they took and drop it with their scope.
func (fr *frame) isExpr(n syntax.NodeID) (Value, *ctrl) {
	node := fr.t.Nodes[n]
	lhs, pat := syntax.NodeID(node.Lhs), syntax.NodeID(node.Rhs)
	v, c := fr.expr(lhs)
	if c != nil {
		return nil, c
	}
	ok := fr.match(pat, v)
	switch {
	case isPlaceExpr(fr.t, lhs):
		if ok {
			fr.markPatternMoved(lhs, pat, v)
		}
	case fr.in.moveOnly(v) && !fr.patternBinds(pat):
		fr.dropDeep(v)
	}
	return Bool(ok), nil
}
