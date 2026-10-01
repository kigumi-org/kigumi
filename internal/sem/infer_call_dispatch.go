package sem

import "kigumi/internal/syntax"

// A call on the right of `|>` takes the left operand first.
func (c *checker) synthCall(n syntax.NodeID, want TypeID) TypeID {
	node := c.t.Nodes[n]
	callee := syntax.NodeID(node.Lhs)
	args := c.t.Children(syntax.NodeID(node.Rhs))
	if p := c.piped[n]; p != 0 {
		args = append([]syntax.NodeID{p}, args...)
	}
	t := c.synthCallWith(n, callee, args, want)
	if p := c.piped[n]; p != 0 {
		ci := c.info.Calls[n]
		ci.Piped = p
		c.info.Calls[n] = ci
	}
	return t
}

func (c *checker) synthCallWith(n, callee syntax.NodeID, args []syntax.NodeID, want TypeID) TypeID {
	head := c.resolveHead(callee)
	switch head.kind {
	case headFn:
		c.info.Uses[callee] = head.ent
		return c.callFn(n, head.ent, head.args, args, want, 0)
	case headSet:
		m := c.pickOverload(n, head.set, args, 0)
		if m == 0 {
			return TyPoison
		}
		c.info.Uses[callee] = m
		return c.callFn(n, m, head.args, args, want, 0)
	case headVariant:
		return c.callVariant(n, head, args, want)
	case headIntrinsic:
		return c.callIntrinsic(n, head.ent, args)
	case headStatic:
		return c.callStatic(n, callee, head, args, want)
	case headType, headParam:
		c.synthArgs(args)
		c.errAt(callee, cTypeAsValue, c.r.Entities[head.ent].Name)
		return TyPoison
	case headPkg:
		c.synthArgs(args)
		c.errAt(callee, cPackageAsValue, c.r.Entities[head.ent].Name, c.r.Entities[head.ent].Name)
		return TyPoison
	case headNone:
		c.synthArgs(args)
		return TyPoison
	}
	if c.t.Kind(callee) == syntax.MemberExpr {
		return c.callMethod(n, callee, args, want)
	}
	if c.t.Kind(callee) == syntax.BracketExpr {
		if t, ok := c.callBracketMethod(n, callee, args, want); ok {
			return t
		}
	}
	ft := c.vars.resolve(c.synth(callee))
	return c.callValue(n, ft, args, want)
}
