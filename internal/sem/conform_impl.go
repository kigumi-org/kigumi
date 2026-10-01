package sem

// checkImpls (pass 6) verifies every `@impl` assertion.
func (r *Result) checkImpls() {
	for id := 1; id < len(r.Entities); id++ {
		e := &r.Entities[id]
		if e.Kind != EntType || e.File == 0 || e.Flags&EfPoison != 0 {
			continue
		}
		info := r.typeDecl(EntityID(id))
		if len(info.Impls) == 0 {
			continue
		}
		self := r.ownerInstance(EntityID(id))
		seen := map[TypeID]bool{}
		for _, impl := range info.Impls {
			if seen[impl.Iface] {
				r.errAt(e.File, impl.Node, cImplDuplicate, impl.Iface)
				continue
			}
			seen[impl.Iface] = true
			r.checkImpl(EntityID(id), self, impl)
		}
	}
}

func (r *Result) checkImpl(id EntityID, self TypeID, impl ImplAssert) {
	e := &r.Entities[id]
	ifaceEnt := r.Types.Node(impl.Iface).Ent
	rangeVis := e.Vis
	if r.covers(rangeVis, r.Entities[ifaceEnt].Vis) {
		rangeVis = r.Entities[ifaceEnt].Vis
	}
	res := r.conforms(self, impl.Iface, e.Pkg)
	if !res.ok {
		if len(r.typeDecl(id).Params) > 0 {
			r.errAt(e.File, impl.Node, cImplGenericUnsat, e.Name, impl.Iface, r.paramNames(r.typeDecl(id).Params), res.missing)
		} else {
			r.errAt(e.File, impl.Node, cImplUnsatisfied, e.Name, impl.Iface, res.missing)
		}
		return
	}
	for _, w := range res.witnesses {
		if !r.covers(r.Entities[w].Vis, rangeVis) {
			r.errAt(e.File, impl.Node, cImplWitnessVisibility, e.Name, impl.Iface, r.visText(rangeVis), r.Entities[w].Name, r.visText(r.Entities[w].Vis))
		}
	}
}
