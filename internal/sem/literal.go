package sem

import (
	"math/big"

	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// literalValue decodes an IntLit/FloatLit (or a negated one) into the
// Literals table and returns its untyped type.
func (c *checker) literalValue(n syntax.NodeID) TypeID {
	if v, ok := c.info.Literals[n]; ok {
		return c.r.untypedOf(v)
	}
	node := c.t.Nodes[n]
	var v constValue
	switch node.Kind {
	case syntax.IntLit:
		i, ok := syntax.ParseInt(c.t.TokText(node.Tok))
		if !ok {
			return TyPoison
		}
		v = constValue{Kind: constInt, Int: i}
	case syntax.FloatLit:
		f, ok := syntax.ParseFloat(c.t.TokText(node.Tok))
		if !ok {
			return TyPoison
		}
		v = constValue{Kind: constFloat, Float: f}
	case syntax.Unary:
		inner := syntax.NodeID(node.Lhs)
		if c.t.Toks[node.Tok].Kind != token.Minus || !isNumericLiteral(c.t, inner) {
			return 0
		}
		c.literalValue(inner)
		iv := c.info.Literals[inner]
		if iv.Kind == constInt {
			v = constValue{Kind: constInt, Int: new(big.Int).Neg(iv.Int)}
		} else {
			v = constValue{Kind: constFloat, Float: new(big.Float).Neg(iv.Float)}
		}
	default:
		return 0
	}
	c.info.Literals[n] = v
	return c.r.untypedOf(v)
}

func isNumericLiteral(t *syntax.Tree, n syntax.NodeID) bool {
	switch t.Kind(n) {
	case syntax.IntLit, syntax.FloatLit:
		return true
	case syntax.Unary:
		node := t.Nodes[n]
		return t.Toks[node.Tok].Kind == token.Minus && isNumericLiteral(t, syntax.NodeID(node.Lhs))
	}
	return false
}

// checkLiteralFits reports whether the literal at n is representable in
// target, and reports the mismatch otherwise.
func (c *checker) checkLiteralFits(n syntax.NodeID, target TypeID) bool {
	v, ok := c.info.Literals[n]
	if !ok || target == TyPoison {
		return true
	}
	tt := c.r.Types
	switch {
	case tt.IsInteger(target) && v.Kind == constInt:
		if !c.r.intFits(v.Int, target) {
			c.errAt(n, cLiteralOutOfRange, v.Int.String(), target)
			return false
		}
	case tt.IsInteger(target) && v.Kind == constFloat:
		c.errAt(n, cLiteralFloatIntoInt, v.Float.Text('g', -1), target)
		return false
	case tt.IsFloat(target):
		if v.Kind == constInt && !intExactAsFloat(v.Int, tt.Width(target)) {
			c.errAt(n, cLiteralOutOfRange, v.Int.String(), target)
			return false
		}
	case tt.Kind(target) == KUntyped:
	default:
		c.errAt(n, cTypeMismatch, target, c.r.untypedOf(v))
		return false
	}
	return true
}

func intExactAsFloat(v *big.Int, bits int) bool {
	mant := 53
	if bits == 32 {
		mant = 24
	}
	f := new(big.Float).SetPrec(uint(mant)).SetInt(v)
	back, _ := f.Int(nil)
	return back.Cmp(v) == 0
}

// fitsQuiet is checkLiteralFits without reporting; used to choose among candidates.
func (c *checker) fitsQuiet(n syntax.NodeID, target TypeID) bool {
	v, ok := c.info.Literals[n]
	if !ok {
		return true
	}
	tt := c.r.Types
	switch {
	case tt.IsInteger(target):
		return v.Kind == constInt && c.r.intFits(v.Int, target)
	case tt.IsFloat(target):
		return v.Kind == constFloat || intExactAsFloat(v.Int, tt.Width(target))
	}
	return false
}

// defaultOf gives the default concrete type of an untyped literal type.
func defaultOf(t TypeID) TypeID {
	switch t {
	case TyUntypedInt:
		return TyI64
	case TyUntypedFloat:
		return TyF64
	}
	return t
}

// adopt gives a literal node the concrete numeric type target (INF-9).
func (c *checker) adopt(n syntax.NodeID, target TypeID) TypeID {
	if c.checkLiteralFits(n, target) {
		return c.setType(n, target)
	}
	return c.setType(n, TyPoison)
}

// literalNode finds the literal node behind an expression whose type is
// untyped: the node itself or a parenthesized literal.
func (c *checker) literalNode(n syntax.NodeID) syntax.NodeID {
	for c.t.Kind(n) == syntax.Paren {
		n = syntax.NodeID(c.t.Nodes[n].Lhs)
	}
	return n
}
