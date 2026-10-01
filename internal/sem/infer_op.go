package sem

import (
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func (c *checker) synthBinary(n syntax.NodeID, want TypeID) TypeID {
	node := c.t.Nodes[n]
	op := c.t.Toks[node.Tok].Kind
	lhs, rhs := syntax.NodeID(node.Lhs), syntax.NodeID(node.Rhs)
	switch op {
	case token.OrOr:
		return c.synthFallback(n, lhs, rhs, want)
	case token.PipeGt:
		return c.synthPipe(n, lhs, rhs)
	case token.AndAnd:
		c.condFacts(n)
		return TyBool
	}
	l := c.readThrough(c.vars.resolve(c.synth(lhs)))
	r := c.readThrough(c.vars.resolve(c.synth(rhs)))
	tt := c.r.Types
	if op == token.DotDot || op == token.DotDotEq {
		return c.synthRange(n, op, lhs, rhs, l, r)
	}
	if folded := c.foldBinary(n, op, lhs, rhs, l, r); folded != 0 {
		return folded
	}
	switch op {
	case token.EqEq, token.NotEq, token.Lt, token.LtEq, token.Gt, token.GtEq:
		l, r = c.adoptPair(lhs, rhs, c.scrutineeOf(l), c.scrutineeOf(r))
		if l == TyPoison || r == TyPoison {
			return TyPoison
		}
		if op == token.EqEq || op == token.NotEq {
			return c.equality(n, lhs, rhs, l, r, op)
		}
		return c.ordering(n, rhs, l, r, op)
	}
	if op == token.Operator || tt.Kind(l) == KNamed || tt.Kind(r) == KNamed {
		return c.userOperator(n, c.t.TokText(node.Tok), []TypeID{l, r}, []syntax.NodeID{lhs, rhs})
	}
	if (op == token.Shl || op == token.Shr) && tt.Kind(r) == KUntyped {
		r = c.adopt(c.literalNode(rhs), TyUsize)
	}
	l, r = c.adoptPair(lhs, rhs, l, r)
	if l == TyPoison || r == TyPoison {
		return TyPoison
	}
	switch op {
	case token.Plus, token.Minus, token.Star, token.Slash, token.Percent:
		if l == r && tt.IsNumeric(l) {
			return c.builtinOp(n, op, l)
		}
	case token.Amp, token.Pipe, token.Caret:
		if l == r && tt.IsInteger(l) {
			return c.builtinOp(n, op, l)
		}
	case token.Shl, token.Shr:
		if tt.IsInteger(l) {
			if r != TyUsize {
				c.errAt(rhs, cShiftCountUsize, r)
				return TyPoison
			}
			return c.builtinOp(n, op, l)
		}
	}
	return c.operandMismatch(n, rhs, c.t.TokText(node.Tok), l, r)
}

func (c *checker) builtinOp(n syntax.NodeID, op token.Kind, result TypeID) TypeID {
	c.info.Calls[n] = CallInfo{Kind: CallBuiltinOp, Op: op, OpText: op.String()}
	return result
}

func (c *checker) operandMismatch(n, rhs syntax.NodeID, op string, l, r TypeID) TypeID {
	tt := c.r.Types
	switch {
	case c.isMethodValue(rhs):
		c.mismatch(rhs, l, r)
	case tt.IsNumeric(l) && tt.IsNumeric(r) && l != r:
		c.errAt(n, cNumericMixed, l, r)
	case l != r && (tt.IsNumeric(l) || l == TyBool || l == TyString || l == TyChar || l == TyBytes):
		c.mismatch(rhs, l, r)
	default:
		c.errAt(n, cOperatorUndefined, op, c.r.TypeString(l)+", "+c.r.TypeString(r))
	}
	return TyPoison
}

func (c *checker) equality(n, lhs, rhs syntax.NodeID, l, r TypeID, op token.Kind) TypeID {
	if l != r && c.vars.unifiable(l, r) {
		c.vars.unify(l, r)
		l = c.vars.resolve(l)
		r = l
	}
	if l != r {
		return c.comparisonMismatch(n, rhs, l, r)
	}
	l = c.vars.settleLiterals(l)
	if !c.r.hasEq(l) {
		if why := c.r.protocolMismatch(l, "Eq"); why != "" {
			c.errAt(n, cProtocolMismatch, l, "equals", "Eq", why)
		} else {
			c.errAt(n, cEqUnsupported, l)
		}
		return TyPoison
	}
	if c.isBareNone(lhs) || c.isBareNone(rhs) {
		place := lhs
		if c.isBareNone(lhs) {
			place = rhs
		}
		c.errAt(n, cEqNoneLint, c.placeName(place))
	}
	if !c.primitiveCompare(l) && c.protoCall(n, l, "Eq", "equals", op) {
		return TyBool
	}
	return c.builtinOp(n, op, TyBool)
}

func (c *checker) ordering(n, rhs syntax.NodeID, l, r TypeID, op token.Kind) TypeID {
	if l != r {
		return c.comparisonMismatch(n, rhs, l, r)
	}
	if !c.r.hasOrd(l) {
		c.errAt(n, cOrdUnsupported, op.String(), l)
		return TyPoison
	}
	if !c.primitiveCompare(l) && c.protoCall(n, l, "Ord", "compareTo", op) {
		return TyBool
	}
	return c.builtinOp(n, op, TyBool)
}
