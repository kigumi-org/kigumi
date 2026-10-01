package sem

// declareWitnesses adds a function's hidden witness parameters, one per
// requirement of each interface constraint on its type parameters.
func (r *Result) declareWitnesses(id EntityID) {
	info := r.Fn(id)
	scope := r.declScope(id)
	seen := map[EntityID]bool{}
	params := append(append([]EntityID{}, info.TypeParams...), info.RecvParams...)
	for _, p := range params {
		if seen[p] {
			continue
		}
		seen[p] = true
		for _, con := range r.typeParam(p).Constraints {
			if con.Kind != CIface {
				continue
			}
			for _, req := range r.witnessReqs(con.Type) {
				local := r.newEntity(Entity{Kind: EntLocal, Name: witnessName(r, p, req), Pkg: r.Entities[id].Pkg, File: r.Entities[id].File, Node: r.Entities[id].Node, Parent: id, Type: r.witnessType(con.Type, r.Types.Param(p), req)})
				r.Entities[local].Detail = r.addLocal(LocalInfo{Scope: scope})
				r.define(scope, r.Entities[local].Name, Binding{Ent: local})
				r.Fn(id).Witnesses = append(r.Fn(id).Witnesses, WitnessSlot{Param: p, Iface: con.Type, Req: req, Local: local})
			}
		}
	}
}

// witnessReqs lists the requirements a witness must carry, in declaration
// order.
func (r *Result) witnessReqs(iface TypeID) []EntityID {
	return r.iface(r.Types.Node(iface).Ent).Reqs
}

// witnessType is the signature of req for self, with the interface's own
// parameters substituted from the constraint.
func (r *Result) witnessType(iface, self TypeID, req EntityID) TypeID {
	in := r.Types.Node(iface)
	info := r.iface(in.Ent)
	subst := map[EntityID]TypeID{info.SelfParam: self}
	for i, p := range info.Params {
		if i < len(in.Args) {
			subst[p] = in.Args[i]
		}
	}
	return r.Types.Subst(r.Fn(req).Sig, subst)
}

func witnessName(r *Result, param, req EntityID) string {
	return "$w:" + r.Entities[param].Name + ":" + r.Entities[req].Name
}

func (r *Result) isMarkerIface(iface EntityID) bool {
	return iface != 0 && (iface == r.langItem(0, 0, "Eq") || iface == r.langItem(0, 0, "Hash") || iface == r.langItem(0, 0, "Ord"))
}

func (r *Result) witnessSlot(fn, param, req EntityID) EntityID {
	for _, w := range r.Fn(fn).Witnesses {
		if w.Param == param && w.Req == req {
			return w.Local
		}
	}
	return 0
}
