package sem

import "kigumi/internal/syntax"

// synthFallback implements `a || b`.
func (c *checker) synthFallback(n, lhs, rhs syntax.NodeID, want TypeID) TypeID {
	tt := c.r.Types
	l := c.vars.resolve(c.synth(lhs))
	if tt.Kind(l) == KUntyped {
		l = c.adopt(c.literalNode(lhs), defaultOf(l))
	}
	if l == TyPoison {
		if c.t.Kind(rhs) != syntax.Lambda {
			c.synth(rhs)
		}
		return TyPoison
	}
	v, r, witness, ok := c.fallbackParts(n, lhs, l)
	if !ok {
		if c.t.Kind(rhs) != syntax.Lambda {
			c.synth(rhs)
		}
		return TyPoison
	}
	c.info.Calls[n] = CallInfo{Kind: CallFallback, Callee: witness}
	c.touched = append(c.touched, n)
	result := v
	if want != 0 {
		steps, ok := c.coerceSteps(lhs, v, want)
		if !ok {
			c.mismatch(n, want, v)
			return TyPoison
		}
		if len(steps) > 0 {
			c.info.Coerce[n] = Coercion{From: v, Steps: steps}
		}
		result = want
	}
	if c.t.Kind(rhs) == syntax.Lambda {
		c.fallbackArm(rhs, r, result)
	} else {
		c.check(rhs, result)
	}
	return result
}

func (c *checker) fallbackParts(n, lhs syntax.NodeID, l TypeID) (v, r TypeID, witness EntityID, ok bool) {
	tt := c.r.Types
	if l == TyBool {
		return TyBool, TyUnit, 0, true
	}
	if elem, isOpt := tt.IsOption(l); isOpt {
		if elem == TyBool {
			c.errAt(n, cFallbackOptionBool)
		}
		return elem, TyUnit, 0, true
	}
	if val, e, isRes := tt.IsResult(l); isRes {
		return val, e, 0, true
	}
	branch := c.r.langItems["Branch"]
	if set := c.r.memberSet(l, "branch"); set != 0 && branch != 0 {
		m := c.r.Overloads[set].Members[0]
		sig, _, _ := c.instantiateFn(n, m, nil, l)
		ret := c.vars.resolve(tt.Node(sig).Elem)
		if c.r.Fn(m).Recv == RecvMove && tt.Kind(ret) == KNamed && tt.Node(ret).Ent == branch {
			c.r.useEntity(c.f, lhs, m)
			c.checkReceiver(lhs, l, m)
			c.edge(EffectEdge{Kind: EdgeCall, Target: m, Node: n})
			return tt.Node(ret).Args[0], tt.Node(ret).Args[1], m, true
		}
	}
	c.errAt(lhs, cFallbackNotSupported, l)
	return 0, 0, 0, false
}

func (c *checker) fallbackArm(lambda syntax.NodeID, r, want TypeID) {
	node := c.t.Nodes[lambda]
	params := c.t.Children(syntax.NodeID(node.Lhs))
	body := syntax.NodeID(node.Rhs)
	if len(params) != 1 {
		c.errAt(lambda, cFallbackArmArity)
		c.setType(lambda, TyPoison)
		return
	}
	p := params[0]
	ps := param(c.t, p)
	if ps.Type != 0 {
		annotated := c.r.resolveType(c.f, c.scope, ps.Type, posLocal)
		if annotated != r && annotated != TyPoison {
			c.errAt(ps.Type, cFallbackArmParamType, r, annotated)
		}
	}
	saved := c.scope
	c.pushScope(ScopeArm, lambda)
	c.declareLocal(p, c.t.TokText(c.t.Nodes[p].Tok), r, false)
	c.setType(p, r)
	c.check(body, want)
	c.popScope(saved)
	c.setType(lambda, want)
}
