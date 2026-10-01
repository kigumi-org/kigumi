package sem

import "kigumi/internal/syntax"

func (c *checker) synthString(n syntax.NodeID) TypeID {
	parts := c.t.StringParts(n)
	interpolated := false
	for _, p := range parts {
		if p.Expr == 0 {
			continue
		}
		interpolated = true
		t := c.readThrough(c.vars.resolve(c.synth(p.Expr)))
		tt := c.r.Types
		switch {
		case t == TyPoison, t == TyString, t == TyChar, t == TyBool, tt.IsNumeric(t), c.vars.pendingLiteral(t):
		case tt.Kind(t) == KUntyped:
			c.adopt(c.literalNode(p.Expr), defaultOf(t))
		default:
			c.errAt(p.Expr, cInterpolationType, t)
		}
	}
	if interpolated {
		c.edge(EffectEdge{Kind: EdgeAlloc, Node: n, Why: 1})
		c.info.Calls[n] = CallInfo{Kind: CallInterp}
	}
	return TyString
}

func (c *checker) synthShell(n syntax.NodeID) TypeID {
	plan := c.r.langItem(c.f, n, "Plan")
	c.shellInterps(syntax.NodeID(c.t.Nodes[n].Rhs))
	if plan == 0 {
		return TyPoison
	}
	return c.r.Types.Named(plan, nil)
}

func (c *checker) shellInterps(n syntax.NodeID) {
	if n == 0 {
		return
	}
	if c.t.Kind(n) == syntax.ShellInterp {
		e := syntax.NodeID(c.t.Nodes[n].Lhs)
		t := c.vars.resolve(c.synth(e))
		tt := c.r.Types
		switch {
		case t == TyPoison, t == TyString, tt.IsInteger(t):
		case tt.Kind(t) == KUntyped && t == TyUntypedInt:
			c.adopt(c.literalNode(e), TyI64)
		case c.r.memberSet(t, "shellArg") != 0:
		default:
			c.errAt(e, cShellInterpType, t)
		}
		return
	}
	c.t.EachChild(n, c.shellInterps)
}

func (c *checker) synthComptime(n syntax.NodeID) TypeID {
	inner := syntax.NodeID(c.t.Nodes[n].Lhs)
	if c.t.Kind(inner) == syntax.Block {
		return c.synthComptimeBlock(n, inner)
	}
	v, ok := c.r.evalConst(c.f, inner)
	if !ok {
		return TyPoison
	}
	if v.Kind == constInt || v.Kind == constFloat {
		c.info.Literals[n] = v
		c.info.Literals[inner] = v
	}
	return c.setType(inner, c.r.untypedOf(v))
}

func (c *checker) synthAllocator(n syntax.NodeID, want TypeID) TypeID {
	node := c.t.Nodes[n]
	scopeType := c.vars.resolve(c.synth(syntax.NodeID(node.Lhs)))
	if item := c.r.langItem(c.f, n, "AllocatorScope"); item != 0 && scopeType != TyPoison {
		if scopeType != c.r.Types.Named(item, nil) {
			c.errAt(syntax.NodeID(node.Lhs), cAllocatorScopeType, scopeType)
		}
	}
	c.finishExpr()
	c.allocDepth++
	t := c.check(syntax.NodeID(node.Rhs), want)
	c.allocDepth--
	return t
}

// finishExpr marks a sub-expression (condition, iterable, scrutinee) as its own full expression.
func (c *checker) finishExpr() {}

// The effects pass reads the EntContract entity created here.
func (c *checker) synthContract(n syntax.NodeID, want TypeID) TypeID {
	node := c.t.Nodes[n]
	id := c.r.newEntity(Entity{Kind: EntContract, Name: "contract", Pkg: c.pkg, File: c.f, Node: n, Parent: c.fn})
	c.r.Entities[id].Detail = c.r.addFn(FnInfo{Declared: Effects(node.Lhs), Body: syntax.NodeID(node.Rhs)})
	c.info.Defs[n] = id
	saved := c.edges
	var edges []EffectEdge
	c.edges = &edges
	c.contracts = append(c.contracts, Effects(node.Lhs))
	t := c.check(syntax.NodeID(node.Rhs), want)
	c.contracts = c.contracts[:len(c.contracts)-1]
	c.edges = saved
	c.r.Fn(id).Edges = edges
	c.edge(EffectEdge{Kind: EdgeCall, Target: id, Node: n})
	return t
}

func (c *checker) synthOptMember(n syntax.NodeID) TypeID {
	node := c.t.Nodes[n]
	base := c.vars.resolve(c.synth(syntax.NodeID(node.Lhs)))
	elem, ok := c.r.Types.IsOption(base)
	if !ok {
		if base != TyPoison {
			c.errAt(n, cOptMemberNotOption, base)
		}
		return TyPoison
	}
	t := c.memberOn(n, syntax.NodeID(node.Lhs), elem, c.t.TokText(node.Tok))
	if t == TyPoison {
		return TyPoison
	}
	return c.r.Types.Option(t)
}
