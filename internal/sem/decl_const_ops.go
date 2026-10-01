package sem

import (
	"math/big"

	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func (r *Result) constUnary(f FileID, n syntax.NodeID, op token.Kind, v constValue) (constValue, bool) {
	switch {
	case op == token.Minus && v.Kind == constInt:
		return constValue{Kind: constInt, Int: new(big.Int).Neg(v.Int)}, true
	case op == token.Minus && v.Kind == constFloat:
		return constValue{Kind: constFloat, Float: new(big.Float).Neg(v.Float)}, true
	case op == token.Bang && v.Kind == constBool:
		return constValue{Kind: constBool, Bool: !v.Bool}, true
	case op == token.Tilde && v.Kind == constInt:
		return constValue{Kind: constInt, Int: new(big.Int).Not(v.Int)}, true
	}
	r.errAt(f, n, cConstNotComptime, "this operator")
	return constValue{}, false
}

func (r *Result) constBinary(f FileID, n syntax.NodeID, op token.Kind, a, b constValue) (constValue, bool) {
	fail := func() (constValue, bool) {
		r.errAt(f, n, cConstNotComptime, "this operator")
		return constValue{}, false
	}
	if a.Kind == constInt && b.Kind == constInt {
		x, y := a.Int, b.Int
		z := new(big.Int)
		switch op {
		case token.Plus:
			z.Add(x, y)
		case token.Minus:
			z.Sub(x, y)
		case token.Star:
			z.Mul(x, y)
		case token.Slash, token.Percent:
			if y.Sign() == 0 {
				r.errAt(f, n, cConstDivZero)
				return constValue{}, false
			}
			if op == token.Slash {
				z.Quo(x, y)
			} else {
				z.Rem(x, y)
			}
		case token.Amp:
			z.And(x, y)
		case token.Pipe:
			z.Or(x, y)
		case token.Caret:
			z.Xor(x, y)
		case token.Shl:
			z.Lsh(x, uint(y.Uint64()))
		case token.Shr:
			z.Rsh(x, uint(y.Uint64()))
		case token.EqEq, token.NotEq, token.Lt, token.LtEq, token.Gt, token.GtEq:
			return constValue{Kind: constBool, Bool: compareResult(op, x.Cmp(y))}, true
		default:
			return fail()
		}
		return constValue{Kind: constInt, Int: z}, true
	}
	if (a.Kind == constInt || a.Kind == constFloat) && (b.Kind == constInt || b.Kind == constFloat) {
		x, y := toFloat(a), toFloat(b)
		z := new(big.Float).SetPrec(256)
		switch op {
		case token.Plus:
			z.Add(x, y)
		case token.Minus:
			z.Sub(x, y)
		case token.Star:
			z.Mul(x, y)
		case token.Slash:
			if y.Sign() == 0 {
				r.errAt(f, n, cConstDivZero)
				return constValue{}, false
			}
			z.Quo(x, y)
		case token.EqEq, token.NotEq, token.Lt, token.LtEq, token.Gt, token.GtEq:
			return constValue{Kind: constBool, Bool: compareResult(op, x.Cmp(y))}, true
		default:
			return fail()
		}
		return constValue{Kind: constFloat, Float: z}, true
	}
	if a.Kind == constBool && b.Kind == constBool {
		switch op {
		case token.AndAnd:
			return constValue{Kind: constBool, Bool: a.Bool && b.Bool}, true
		case token.OrOr:
			return constValue{Kind: constBool, Bool: a.Bool || b.Bool}, true
		case token.EqEq:
			return constValue{Kind: constBool, Bool: a.Bool == b.Bool}, true
		case token.NotEq:
			return constValue{Kind: constBool, Bool: a.Bool != b.Bool}, true
		}
	}
	if a.Kind == constString && b.Kind == constString && (op == token.EqEq || op == token.NotEq) {
		return constValue{Kind: constBool, Bool: (a.Str == b.Str) == (op == token.EqEq)}, true
	}
	return fail()
}

func toFloat(v constValue) *big.Float {
	if v.Kind == constFloat {
		return v.Float
	}
	return new(big.Float).SetPrec(256).SetInt(v.Int)
}

func compareResult(op token.Kind, c int) bool {
	switch op {
	case token.EqEq:
		return c == 0
	case token.NotEq:
		return c != 0
	case token.Lt:
		return c < 0
	case token.LtEq:
		return c <= 0
	case token.Gt:
		return c > 0
	}
	return c >= 0
}
