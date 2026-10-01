package sem

import "kigumi/internal/syntax"

func (c *checker) synthArgs(args []syntax.NodeID) {
	for _, a := range args {
		if c.t.Kind(a) == syntax.Spread || c.t.Kind(a) == syntax.NamedArg {
			c.synth(syntax.NodeID(c.t.Nodes[a].Lhs))
			continue
		}
		c.synth(a)
	}
}

func (c *checker) callFn(n syntax.NodeID, fn EntityID, typeArgs []TypeID, args []syntax.NodeID, want TypeID, recv TypeID) TypeID {
	info := c.r.Fn(fn)
	name := c.r.Entities[fn].Name
	async := info.Declared&EffAsync != 0
	if async {
		want = 0
	}
	sig, inst, badTypeArgs := c.instantiateFn(n, fn, typeArgs, recv)
	c.edge(EffectEdge{Kind: EdgeCall, Target: fn, Node: n})
	c.callInvalidates()
	if (info.Declared&EffUnsafe != 0 || info.Abi != "" || info.Naked) && c.unsafe == 0 {
		c.errAt(n, cUnsafeRequired, "calling `"+name+"`")
	}
	if ret, done := c.dlCall(n, fn, sig, inst, recv, args, want); done {
		return ret
	}
	kind := CallFn
	if info.Recv != RecvNone {
		kind = CallMethod
	} else if info.Owner != 0 {
		kind = CallAssoc
	}
	ret, order := c.checkArgs(n, name, sig, args, inst, want, info.Params)
	c.ffiCValueCheck(n, fn, inst, recv)
	c.errorAsCheck(n, fn, inst)
	if async {
		// A direct call to an async fn yields a lazy Future.
		ret = c.r.Types.Named(c.r.Types.futureEnt, []TypeID{ret})
	}
	effArgs := args
	if order != nil {
		effArgs = order
	}
	c.callbackEdges(n, fn, sig, effArgs)
	c.fieldCallbackEdges(fn, effArgs)
	call := CallInfo{Kind: kind, Callee: fn, Inst: inst, Recv: info.Recv, Variadic: c.variadicForm(effArgs, sig), ArgOrder: order}
	c.info.Calls[n] = call
	// badTypeArgs already reported cTypeArgCount; inst is made-up, so skip
	// to avoid a second diagnostic for the same mistake.
	if !badTypeArgs {
		c.deferWitnesses(n, fn, recv, inst)
	}
	c.touched = append(c.touched, n)
	return ret
}

// declParams may be nil (no declaration backs sig, e.g. a bare fn value).
// The second return value is args reordered to parameter position, or nil if unchanged.
func (c *checker) checkArgs(n syntax.NodeID, name string, sig TypeID, args []syntax.NodeID, inst []TypeID, want TypeID, declParams []EntityID) (TypeID, []syntax.NodeID) {
	tt := c.r.Types
	sn := tt.Node(sig)
	params := sn.Args
	cVariadic := sn.Flags&fnCVariadic != 0
	variadic := sn.Flags&fnVariadic != 0
	fixed := len(params)
	if variadic {
		fixed--
	}
	spread := len(args) > 0 && c.t.Kind(args[len(args)-1]) == syntax.Spread
	switch {
	case cVariadic && len(args) < fixed:
		c.errAt(n, cCallArity, name, fixed, plural(fixed), len(args))
		return c.abandonCall(sig, args), nil
	case !cVariadic && !variadic && len(args) != len(params):
		c.errAt(n, cCallArity, name, len(params), plural(len(params)), len(args))
		return c.abandonCall(sig, args), nil
	case !cVariadic && variadic && len(args) < fixed:
		c.errAt(n, cCallArity, name, fixed, plural(fixed), len(args))
		return c.abandonCall(sig, args), nil
	case !cVariadic && variadic && spread && len(args) != fixed+1:
		c.errAt(args[len(args)-1], cVariadicMixed)
		return c.abandonCall(sig, args), nil
	}
	var order []syntax.NodeID
	if c.hasNamedArg(args) {
		reordered, ok := c.resolveNamedArgs(name, args, declParams, fixed)
		if !ok {
			return c.abandonCall(sig, args), nil
		}
		args, order = reordered, reordered
	}
	if cVariadic {
		return c.checkCArgs(n, name, sn, args), order
	}
	for pass := 0; pass < 2; pass++ {
		for i, a := range args {
			if (c.t.Kind(a) == syntax.Lambda) != (pass == 1) {
				continue
			}
			if pass == 1 {
				c.markImmediate(a)
			}
			if c.t.Kind(a) == syntax.Spread {
				if !variadic {
					c.errAt(a, cSpreadNonVariadic, name)
					c.synth(syntax.NodeID(c.t.Nodes[a].Lhs))
					continue
				}
				c.checkArg(syntax.NodeID(c.t.Nodes[a].Lhs), params[fixed], entAt(declParams, fixed))
				continue
			}
			pi := min(i, len(params)-1)
			param := params[pi]
			if variadic && i >= fixed {
				param = tt.Node(params[fixed]).Args[0]
				pi = fixed
			}
			c.checkArg(a, param, entAt(declParams, pi))
		}
	}
	tailSpread := len(args) > 0 && c.t.Kind(args[len(args)-1]) == syntax.Spread
	if variadic && !tailSpread && len(args) > fixed {
		c.edge(EffectEdge{Kind: EdgeAlloc, Node: n, Why: 2})
	}
	ret := sn.Elem
	if want != 0 && tt.ContainsVar(c.vars.resolve(ret)) {
		c.unifyWant(n, ret, want)
	}
	return c.vars.resolve(ret), order
}
