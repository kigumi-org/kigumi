package sem

import (
	"slices"

	"kigumi/internal/syntax"
)

// Differing signatures for the same name are an ambiguity error, not a
// silent first pick; dedup keys on interface instantiation, not the requirement entity.
func (c *checker) ifaceMember(n syntax.NodeID, t TypeID, name string) (req EntityID, sig TypeID, ambiguous bool) {
	tt := c.r.Types
	node := tt.Node(t)
	var ifaces []TypeID
	switch node.Kind {
	case KParam:
		for _, con := range c.r.typeParam(node.Ent).Constraints {
			if con.Kind == CIface {
				ifaces = append(ifaces, con.Type)
			}
		}
	case KIface:
		ifaces = append(ifaces, t)
	}
	type found struct {
		req   EntityID
		sig   TypeID
		iface TypeID
	}
	var founds []found
	for _, it := range ifaces {
		in := tt.Node(it)
		info := c.r.iface(in.Ent)
		req := c.r.requirement(in.Ent, name)
		if req == 0 {
			continue
		}
		if slices.ContainsFunc(founds, func(f found) bool { return f.iface == it }) {
			continue
		}
		subst := map[EntityID]TypeID{info.SelfParam: t}
		for i, p := range info.Params {
			if i < len(in.Args) {
				subst[p] = in.Args[i]
			}
		}
		founds = append(founds, found{req, tt.Subst(c.r.Fn(req).Sig, subst), it})
	}
	switch len(founds) {
	case 0:
		return 0, 0, false
	case 1:
		return founds[0].req, founds[0].sig, false
	}
	for _, f := range founds[1:] {
		if !c.r.sameRequirementSig(founds[0].req, founds[0].sig, f.req, f.sig) {
			c.errAt(n, cAmbiguousMember, name, founds[0].iface, f.iface)
			return 0, TyPoison, true
		}
	}
	return founds[0].req, founds[0].sig, false
}

func (c *checker) callRequirement(n, callee, base syntax.NodeID, recv TypeID, req EntityID, sig TypeID, args []syntax.NodeID, want TypeID) TypeID {
	c.info.Uses[callee] = req
	c.checkReceiver(base, recv, req)
	var paramEnt EntityID
	if valueType := c.r.Types.Node(recv); c.r.Types.Kind(recv) == KParam || c.r.Types.Kind(recv) == KRef && c.r.Types.Kind(valueType.Elem) == KParam {
		param := recv
		if c.r.Types.Kind(recv) == KRef {
			param = valueType.Elem
		}
		paramEnt = c.r.Types.Node(param).Ent
	}
	// An existential receiver has no witness to defer to, so the call stays impure regardless of `pure?`.
	if paramEnt != 0 && c.r.Fn(req).EffectPoly {
		c.deferToWitnessReq(paramEnt, req)
		c.edge(EffectEdge{Kind: EdgeWitnessCall, Target: req, Node: n})
	} else {
		c.edge(EffectEdge{Kind: EdgeCall, Target: req, Node: n})
	}
	ret, order := c.checkArgs(n, c.r.Entities[req].Name, sig, args, nil, want, c.r.Fn(req).Params)
	effArgs := args
	if order != nil {
		effArgs = order
	}
	if c.r.Fn(req).Recv == RecvMut {
		c.checkReentrantCapture(base, effArgs)
	}
	call := CallInfo{Kind: CallMethod, Callee: req, Recv: c.r.Fn(req).Recv, Variadic: c.variadicForm(effArgs, sig), ArgOrder: order}
	if paramEnt != 0 {
		call.Witness = c.witnessLocal(n, paramEnt, req)
	}
	c.info.Calls[n] = call
	c.touched = append(c.touched, n)
	return ret
}

// Types `v.m` without a call: a fn value that copies its receiver (LAM-5).
func (c *checker) methodValue(n, base syntax.NodeID, valueType TypeID, m EntityID) TypeID {
	info := c.r.Fn(m)
	name := c.r.Entities[m].Name
	c.info.Uses[n] = m
	c.info.Calls[n] = CallInfo{Kind: CallMethodValue, Callee: m, Recv: info.Recv}
	sig := c.r.memberSig(valueType, m)
	switch {
	case c.r.Types.Node(sig).Flags&fnVariadic != 0:
		c.errAt(n, cVariadicNoValue, name, name)
		return TyPoison
	case len(c.ownGenerics(m)) > 0:
		sig, _, _ = c.instantiateFn(n, m, nil, valueType)
		c.touched = append(c.touched, n)
	case !c.r.isCopy(valueType):
		c.errAt(n, cMethodValueMoveOnly, valueType)
		return TyPoison
	}
	if info.Declared&EffAsync != 0 {
		sig = c.asyncValueFn(sig)
	}
	c.edge(EffectEdge{Kind: EdgeCall, Target: m, Node: n})
	return sig
}

func (c *checker) isMethodValue(n syntax.NodeID) bool {
	call, ok := c.info.Calls[c.literalNode(n)]
	return ok && call.Kind == CallMethodValue
}

// Resolves to the width that uniquely declares name, else the untyped default
// always picks a concrete type so a missing member names it like a bound variable would.
func (c *checker) resolveLiteralReceiver(untyped TypeID, name string) (TypeID, bool) {
	var pool []TypeID
	switch untyped {
	case TyUntypedInt:
		for t := TyI8; t <= TyIsize; t++ {
			pool = append(pool, t)
		}
	case TyUntypedFloat:
		pool = []TypeID{TyF32, TyF64}
	default:
		return 0, false
	}
	var only TypeID
	for _, t := range pool {
		if c.r.memberSet(t, name) != 0 {
			if only != 0 {
				only = 0
				break
			}
			only = t
		}
	}
	if only != 0 {
		return only, true
	}
	return defaultOf(untyped), true
}

func (c *checker) overloadValue(n syntax.NodeID, want TypeID) (TypeID, bool) {
	head := c.resolveHead(n)
	if head.kind != headSet {
		return 0, false
	}
	var match []EntityID
	for _, m := range c.r.Overloads[head.set].Members {
		if c.sameFnShape(c.r.Fn(m).Sig, want) {
			match = append(match, m)
		}
	}
	if len(match) != 1 {
		c.errAt(n, cOverloadValueAmbig, c.r.Overloads[head.set].Name)
		return TyPoison, true
	}
	c.r.useEntity(c.f, n, match[0])
	c.info.Uses[n] = match[0]
	return c.r.Fn(match[0]).Sig, true
}
