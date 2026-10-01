package interp

import (
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func (fr *frame) intOp(n syntax.NodeID, op token.Kind, a, b Int) Value {
	in := fr.in
	signed := in.r.Types.IsSigned(a.T)
	switch op {
	case token.Plus:
		s := a.V + b.V
		over := signed && ((a.V >= 0) == (b.V >= 0)) && ((s >= 0) != (a.V >= 0))
		if !signed && bitsOf(in, a.T) == 64 {
			over = uint64(s) < uint64(a.V)
		}
		return fr.checkedInt(n, s, a.T, over)
	case token.Minus:
		s := a.V - b.V
		over := signed && ((a.V >= 0) != (b.V >= 0)) && ((s >= 0) != (a.V >= 0))
		if !signed {
			over = uint64(a.V) < uint64(b.V)
		}
		return fr.checkedInt(n, s, a.T, over)
	case token.Star:
		p := a.V * b.V
		over := a.V != 0 && (p/a.V != b.V || a.V == -1 && b.V == minOf(in, a.T))
		if !signed && bitsOf(in, a.T) == 64 && a.V != 0 {
			over = uint64(p)/uint64(a.V) != uint64(b.V)
		}
		return fr.checkedInt(n, p, a.T, over)
	case token.Slash, token.Percent:
		if b.V == 0 {
			fr.panicAt(n, "division by zero")
		}
		if signed && b.V == -1 && a.V == minOf(in, a.T) {
			fr.panicAt(n, "integer overflow")
		}
		if !signed {
			if op == token.Slash {
				return Int{V: int64(uint64(a.V) / uint64(b.V)), T: a.T}
			}
			return Int{V: int64(uint64(a.V) % uint64(b.V)), T: a.T}
		}
		if op == token.Slash {
			return Int{V: a.V / b.V, T: a.T}
		}
		return Int{V: a.V % b.V, T: a.T}
	case token.Amp:
		return Int{V: a.V & b.V, T: a.T}
	case token.Pipe:
		return Int{V: a.V | b.V, T: a.T}
	case token.Caret:
		return Int{V: truncate(a.V^b.V, a.T, in), T: a.T}
	case token.Shl, token.Shr:
		if b.V < 0 || b.V >= int64(bitsOf(in, a.T)) {
			fr.panicAt(n, "shift count %d out of range", b.V)
		}
		if op == token.Shl {
			return Int{V: truncate(a.V<<b.V, a.T, in), T: a.T}
		}
		if signed {
			return Int{V: a.V >> b.V, T: a.T}
		}
		return Int{V: int64(uint64(a.V) >> b.V), T: a.T}
	case token.Lt, token.LtEq, token.Gt, token.GtEq:
		c := cmpInt(a.V, b.V)
		if !signed {
			switch {
			case uint64(a.V) < uint64(b.V):
				c = -1
			case uint64(a.V) > uint64(b.V):
				c = 1
			default:
				c = 0
			}
		}
		return Bool(compareOp(op, c))
	}
	fr.panicAt(n, "bad integer operator %v (%s)", op, fr.t.TokText(fr.t.Nodes[n].Tok))
	return nil
}

func floatOp(in *Interp, op token.Kind, a, b Float) Value {
	switch op {
	case token.Plus:
		return mkFloat(in, a.V+b.V, a.T)
	case token.Minus:
		return mkFloat(in, a.V-b.V, a.T)
	case token.Star:
		return mkFloat(in, a.V*b.V, a.T)
	case token.Slash:
		return mkFloat(in, a.V/b.V, a.T)
	case token.Lt:
		return Bool(a.V < b.V)
	case token.LtEq:
		return Bool(a.V <= b.V)
	case token.Gt:
		return Bool(a.V > b.V)
	case token.GtEq:
		return Bool(a.V >= b.V)
	}
	return Unit{}
}
