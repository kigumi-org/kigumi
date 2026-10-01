package sem

import "kigumi/internal/syntax"

// WitnessSlot is one hidden parameter: the implementation of requirement
// Req of interface instance Iface for type parameter Param, bound to the
// local Local inside the body.
type WitnessSlot struct {
	Param EntityID
	Iface TypeID
	Req   EntityID
	Local EntityID
}

// witnessArg resolves how the caller supplies req for t: a hidden local
// for a type parameter, otherwise the implementing function.
func (c *checker) witnessArg(n syntax.NodeID, t, iface TypeID, req EntityID) WitnessArg {
	tt := c.r.Types
	if tt.Kind(t) == KParam {
		return WitnessArg{Kind: WitnessLocal, Ent: c.witnessLocal(n, tt.Node(t).Ent, req)}
	}
	ifaceEnt := tt.Node(iface).Ent
	m, _, _, ownSubst := c.r.findWitness(t, req, c.r.ifaceInstSubst(iface, t), c.pkg)
	if m == 0 {
		// A false from requestDerive is final: it recomputes the same way
		// again. alreadyUnsatisfied avoids re-reporting a failure
		// checkConstraints already flagged.
		if !c.r.requestDerive(t, iface) && !c.alreadyUnsatisfied(n, t, iface) {
			if why := c.r.protocolMismatch(t, c.r.Entities[ifaceEnt].Name); why != "" {
				c.errAt(n, cProtocolMismatch, t, c.r.Entities[req].Name, c.r.Entities[ifaceEnt].Name, why)
			} else {
				c.errAt(n, cWitnessMissing, t, c.r.Entities[req].Name, c.r.Entities[ifaceEnt].Name)
			}
		}
		return WitnessArg{}
	}
	out := WitnessArg{Kind: WitnessFn, Ent: m}
	slots := c.r.Fn(m).Witnesses
	constParams := c.r.constParamsOf(m)
	if len(slots) > 0 || len(constParams) > 0 {
		out.Kind = WitnessBind
		subst := map[EntityID]TypeID{}
		tn := tt.Node(t)
		if tn.Kind == KNamed {
			for i, p := range c.r.typeDecl(tn.Ent).Params {
				if i < len(tn.Args) {
					subst[p] = tn.Args[i]
				}
			}
			for i, p := range c.r.Fn(m).RecvParams {
				if i < len(tn.Args) {
					subst[p] = tn.Args[i]
				}
			}
		}
		// ownSubst carries m's own type parameters (e.g. T in `fn
		// W.render[T: Show]`) to the type they were unified to.
		for p, a := range ownSubst {
			subst[p] = a
		}
		for _, slot := range slots {
			out.Args = append(out.Args, c.witnessArg(n, tt.Subst(tt.Param(slot.Param), subst), tt.Subst(slot.Iface, subst), slot.Req))
		}
		for _, p := range constParams {
			out.ConstArgs = append(out.ConstArgs, c.constArgFromType(n, tt.Subst(tt.Param(p), subst)))
		}
	}
	return out
}

// witnessIfaceSubst binds an interface's Self (and, for now, nothing
// else) when looking up a witness for t.
func (c *checker) witnessIfaceSubst(iface EntityID, t TypeID) map[EntityID]TypeID {
	return map[EntityID]TypeID{c.r.iface(iface).SelfParam: t}
}
