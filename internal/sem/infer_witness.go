package sem

import "kigumi/internal/syntax"

// paramMember resolves `T.name` on a type parameter: a static requirement
// of one of its interface constraints.
func (c *checker) paramMember(n syntax.NodeID, base headRef, name string) headRef {
	for _, con := range c.r.typeParam(base.ent).Constraints {
		if con.Kind != CIface {
			continue
		}
		req := c.r.requirement(c.r.Types.Node(con.Type).Ent, name)
		if req == 0 || c.r.Fn(req).Recv != RecvNone {
			continue
		}
		c.info.Uses[n] = req
		return headRef{kind: headStatic, ent: req, param: base.ent, iface: con.Type}
	}
	c.errAt(n, cParamNoMember, c.r.Types.Param(base.ent), name)
	return headRef{kind: headNone}
}

// callStatic checks `T.req(args)`: the requirement's signature with Self
// bound to the parameter, dispatched through the hidden witness local.
func (c *checker) callStatic(n, callee syntax.NodeID, head headRef, args []syntax.NodeID, want TypeID) TypeID {
	req := head.ent
	self := c.r.Types.Param(head.param)
	sig := c.r.witnessType(head.iface, self, req)
	local := c.witnessLocal(n, head.param, req)
	if c.r.Fn(req).EffectPoly {
		c.deferToWitnessReq(head.param, req)
		c.edge(EffectEdge{Kind: EdgeWitnessCall, Target: req, Node: n})
	} else {
		c.edge(EffectEdge{Kind: EdgeCall, Target: req, Node: n})
	}
	ret, order := c.checkArgs(n, c.r.Entities[req].Name, sig, args, nil, want, c.r.Fn(req).Params)
	effArgs := args
	if order != nil {
		effArgs = order
	}
	c.info.Calls[n] = CallInfo{Kind: CallStatic, Callee: req, Inst: []TypeID{self}, Witness: local, Variadic: c.variadicForm(effArgs, sig), ArgOrder: order}
	c.touched = append(c.touched, n)
	return ret
}

// witnessLocal finds the hidden local carrying req for param in the
// enclosing function, capturing it into any lambda in between.
func (c *checker) witnessLocal(n syntax.NodeID, param, req EntityID) EntityID {
	b, found, ok := c.r.lookup(c.scope, witnessName(c.r, param, req))
	if !ok || b.Ent == 0 {
		return 0
	}
	c.noteCapture(n, b.Ent, found)
	return b.Ent
}

type witnessTask struct {
	node syntax.NodeID
	fn   EntityID
	self TypeID
	inst []TypeID
	// scope and lambdas are the call site's, so a witness local found
	// later is looked up and captured from where the call was written.
	scope   ScopeID
	lambdas []*lambdaCtx
}

// deferWitnesses schedules the witness arguments of a call for when its
// type arguments are known.
func (c *checker) deferWitnesses(n syntax.NodeID, fn EntityID, self TypeID, inst []TypeID) {
	if len(c.r.Fn(fn).Witnesses) == 0 && len(c.r.Fn(fn).ConstParams) == 0 {
		return
	}
	c.witnessTasks = append(c.witnessTasks, witnessTask{node: n, fn: fn, self: self, inst: inst, scope: c.scope, lambdas: append([]*lambdaCtx(nil), c.lambdas...)})
}

// resolveWitnesses fills CallInfo.Passes for settled calls.
func (c *checker) resolveWitnesses() {
	pending := c.witnessTasks
	c.witnessTasks = nil
	for _, w := range pending {
		subst := c.witnessSubst(w)
		if subst == nil {
			c.witnessTasks = append(c.witnessTasks, w)
			continue
		}
		call := c.info.Calls[w.node]
		call.Passes, call.ConstArgs = nil, nil
		savedScope, savedLambdas := c.scope, c.lambdas
		c.scope, c.lambdas = w.scope, w.lambdas
		for _, slot := range c.r.Fn(w.fn).Witnesses {
			t := c.r.Types.Subst(c.r.Types.Param(slot.Param), subst)
			iface := c.r.Types.Subst(slot.Iface, subst)
			call.Passes = append(call.Passes, c.witnessArg(w.node, t, iface, slot.Req))
		}
		for _, p := range c.r.constParamsOf(w.fn) {
			call.ConstArgs = append(call.ConstArgs, c.constArgFromType(w.node, c.r.Types.Subst(c.r.Types.Param(p), subst)))
		}
		for _, ctx := range w.lambdas {
			info := c.r.closure(ctx.ent)
			info.Captures, info.Copy = ctx.list, closureCopy(ctx.list)
		}
		c.deferWitnessCallback(w.fn, w.node, call.Passes)
		c.scope, c.lambdas = savedScope, savedLambdas
		c.info.Calls[w.node] = call
	}
}

// witnessSubst maps the callee's type parameters to the call's resolved
// arguments; nil while inference is still open.
func (c *checker) witnessSubst(w witnessTask) map[EntityID]TypeID {
	info := c.r.Fn(w.fn)
	subst := map[EntityID]TypeID{}
	if w.self != 0 {
		self := c.vars.resolve(w.self)
		if c.r.Types.ContainsVar(self) {
			return nil
		}
		sn := c.r.Types.Node(self)
		for i, p := range c.r.ownerParams(w.fn) {
			if i < len(sn.Args) {
				subst[p] = sn.Args[i]
			}
		}
		for i, p := range info.RecvParams {
			if i < len(sn.Args) {
				subst[p] = sn.Args[i]
			}
		}
	}
	own := info.TypeParams
	if w.self != 0 && len(c.r.ownerParams(w.fn)) > 0 && len(own) >= len(c.r.ownerParams(w.fn)) {
		own = own[len(c.r.ownerParams(w.fn)):]
	}
	for i, p := range own {
		if i >= len(w.inst) {
			break
		}
		t := c.vars.resolve(w.inst[i])
		if c.r.Types.ContainsVar(t) {
			return nil
		}
		subst[p] = t
	}
	return subst
}
