package sem

import "kigumi/internal/syntax"

// synthPipe types `a |> b`.
func (c *checker) synthPipe(n, lhs, rhs syntax.NodeID) TypeID {
	target := rhs
	if c.t.Kind(target) == syntax.TryExpr {
		target = syntax.NodeID(c.t.Nodes[target].Lhs)
	}
	switch c.t.Kind(target) {
	case syntax.CallExpr, syntax.WsCallExpr:
		if c.piped == nil {
			c.piped = map[syntax.NodeID]syntax.NodeID{}
		}
		c.piped[target] = lhs
		t := c.synth(rhs)
		c.info.Calls[n] = CallInfo{Kind: CallPipe}
		return t
	case syntax.Ident, syntax.MemberExpr, syntax.BracketExpr:
		if target != rhs || !c.pathLike(rhs) {
			break
		}
		t := c.synthCallWith(n, rhs, []syntax.NodeID{lhs}, 0)
		ci := c.info.Calls[n]
		ci.Piped = lhs
		c.info.Calls[n] = ci
		return t
	case syntax.Lambda:
		if target != rhs {
			break
		}
		return c.pipeLambda(n, lhs, rhs)
	}
	// The right side is not typed: a call inside it would be checked
	// without the piped argument and report a second, misleading error.
	c.synth(lhs)
	c.errAt(rhs, cPipeNotCall)
	return TyPoison
}

func (c *checker) pipeLambda(n, lhs, rhs syntax.NodeID) TypeID {
	tt := c.r.Types
	c.markImmediate(rhs)
	a := c.vars.resolve(c.synth(lhs))
	if tt.Kind(a) == KUntyped {
		a = c.adopt(c.literalNode(lhs), defaultOf(a))
	}
	if a == TyPoison {
		return TyPoison
	}
	ret := c.vars.fresh(rhs, "return")
	c.check(rhs, tt.Fn([]TypeID{a}, ret, 0, false))
	if c.info.Types[rhs] == TyPoison {
		c.vars.poison(ret)
		return TyPoison
	}
	c.info.Calls[n] = CallInfo{Kind: CallPipe}
	c.touched = append(c.touched, n)
	return c.vars.resolve(ret)
}

func (c *checker) pathLike(n syntax.NodeID) bool {
	switch c.t.Kind(n) {
	case syntax.Ident:
		return true
	case syntax.MemberExpr, syntax.BracketExpr:
		return c.pathLike(syntax.NodeID(c.t.Nodes[n].Lhs))
	}
	return false
}
