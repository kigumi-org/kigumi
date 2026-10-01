package sem

// SelfType is the type a declaration denotes applied to its own type
// parameters; 0 for entities that are neither types nor interfaces.
func (r *Result) SelfType(id EntityID) TypeID {
	e := &r.Entities[id]
	switch e.Kind {
	case EntType:
		if e.Type != 0 {
			return e.Type
		}
		return r.Types.Named(id, r.paramTypes(r.typeDecl(id).Params))
	case EntInterface:
		return r.Types.Iface(id, r.paramTypes(r.iface(id).Params))
	}
	return 0
}

func (r *Result) paramTypes(params []EntityID) []TypeID {
	out := make([]TypeID, len(params))
	for i, p := range params {
		out[i] = r.Types.Param(p)
	}
	return out
}

// Implements reports whether t conforms to iface as seen from pkg and, when
// it does, the method witnessing each requirement in Iface(...).Reqs order.
func (r *Result) Implements(t, iface TypeID, from PackageID) ([]EntityID, bool) {
	res := r.conforms(t, iface, from)
	return res.witnesses, res.ok
}
