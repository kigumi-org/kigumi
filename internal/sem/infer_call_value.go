package sem

import "kigumi/internal/syntax"

func (c *checker) callValue(n syntax.NodeID, ft TypeID, args []syntax.NodeID, want TypeID) TypeID {
	tt := c.r.Types
	if ft == TyPoison {
		c.synthArgs(args)
		return TyPoison
	}
	node := tt.Node(ft)
	var closure EntityID
	if node.Kind == KClosure {
		closure = node.Ent
		ft = c.vars.resolve(c.r.closure(node.Ent).Sig)
		node = tt.Node(ft)
	}
	if node.Kind == KParam {
		for _, con := range c.r.typeParam(node.Ent).Constraints {
			if con.Kind == CFn || con.Kind == CFnMut || con.Kind == CFnOnce {
				ft = con.Type
				node = tt.Node(ft)
				break
			}
		}
	}
	if node.Kind != KFn {
		c.synthArgs(args)
		c.errAt(n, cNotCallable, ft)
		return TyPoison
	}
	c.callInvalidates()
	if fact, ok := c.fieldFact[n]; ok {
		delete(c.fieldFact, n)
		c.applyFieldFact(n, fact)
	} else if closure != 0 {
		c.edge(EffectEdge{Kind: EdgeCall, Target: closure, Node: n})
	} else {
		c.callbackEdge(n, Effects(node.Flags), 0)
	}
	if node.Flags&uint16(EffUnsafe) != 0 && c.unsafe == 0 {
		c.errAt(n, cUnsafeRequired, "calling an `unsafe fn` value")
	}
	if node.Flags&fnCAbi != 0 && c.unsafe == 0 {
		c.errAt(n, cUnsafeRequired, "calling a C function pointer")
	}
	ret, order := c.checkArgs(n, "this function", ft, args, nil, want, nil)
	effArgs := args
	if order != nil {
		effArgs = order
	}
	c.info.Calls[n] = CallInfo{Kind: CallFnValue, Variadic: c.variadicForm(effArgs, ft), ArgOrder: order}
	return ret
}
