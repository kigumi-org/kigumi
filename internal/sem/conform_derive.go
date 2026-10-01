package sem

// Structural derivation of Eq, Hash and Ord: a
// witness method wins, otherwise primitives and aggregates of derivable
// components qualify.

func (r *Result) hasEq(t TypeID) bool   { return r.derive(t, "Eq", &r.Types.props[t].eq) }
func (r *Result) hasHash(t TypeID) bool { return r.derive(t, "Hash", &r.Types.props[t].hash) }
func (r *Result) hasOrd(t TypeID) bool  { return r.derive(t, "Ord", &r.Types.props[t].ord) }

func (r *Result) derive(t TypeID, proto string, memo *int8) bool {
	switch *memo {
	case 1:
		return true
	case -1:
		return false
	}
	*memo = 1
	ok := r.computeDerive(t, proto)
	if !ok {
		*memo = -1
	}
	return ok
}

func (r *Result) computeDerive(t TypeID, proto string) bool {
	tt := r.Types
	n := tt.Node(t)
	switch n.Kind {
	case KPoison, KUntyped:
		return true
	case KPrim:
		return true
	case KParam:
		want := r.langItem(0, 0, proto)
		for _, c := range r.typeParam(n.Ent).Constraints {
			if c.Kind == CIface && tt.Node(c.Type).Ent == want {
				return true
			}
		}
		return false
	case KNamed:
	default:
		return false
	}
	iface := r.langItem(0, 0, proto)
	if iface != 0 && r.conforms(t, tt.Iface(iface, nil), r.Entities[n.Ent].Pkg).ok {
		return true
	}
	info := r.typeDecl(n.Ent)
	// A method of the protocol's name that does not conform is the type's
	// own (wrong) answer; nothing is derived over it.
	if set, ok := info.Members[deriveMethods[proto]]; proto != "Ord" && ok && len(r.Overloads[set].Members) > 0 {
		return false
	}
	if proto == "Ord" {
		return false
	}
	defer func() {
		if r.Types.props[t].eq != -1 && r.derivable(t, proto) {
			r.requestDerive(t, tt.Iface(iface, nil))
		}
	}()
	if info.Form == FormOpaque || info.Form == FormResource {
		if n.Ent != tt.arrayEnt {
			return false
		}
	}
	subst := map[EntityID]TypeID{}
	for i, p := range info.Params {
		if i < len(n.Args) {
			subst[p] = n.Args[i]
		}
	}
	for _, a := range n.Args {
		if !r.deriveOf(a, proto) {
			return false
		}
	}
	for _, fld := range info.Fields {
		if !r.deriveOf(tt.Subst(r.Entities[fld].Type, subst), proto) {
			return false
		}
	}
	for _, v := range info.Variants {
		for _, p := range r.variant(v).Payload {
			if !r.deriveOf(tt.Subst(p, subst), proto) {
				return false
			}
		}
	}
	return true
}

func (r *Result) deriveOf(t TypeID, proto string) bool {
	switch proto {
	case "Hash":
		return r.hasHash(t)
	case "Ord":
		return r.hasOrd(t)
	}
	return r.hasEq(t)
}

// satisfies reports whether t meets a constraint and names what is
// missing otherwise.
func (r *Result) satisfies(t TypeID, c Constraint, from PackageID) (bool, string) {
	switch c.Kind {
	case CCopy:
		if r.isCopy(t) {
			return true, ""
		}
		return false, "it is not `Copy`"
	case CSend:
		if r.isSend(t) {
			return true, ""
		}
		return false, "it is not `Send`"
	case CSync:
		if r.isSync(t) {
			return true, ""
		}
		return false, "it is not `Sync`"
	case CIface:
		ent := r.Types.Node(c.Type).Ent
		switch r.Entities[ent].Name {
		case "Eq":
			if r.Entities[ent].File == 0 && r.hasEq(t) {
				return true, ""
			}
		case "Hash":
			if r.Entities[ent].File == 0 && r.hasHash(t) {
				return true, ""
			}
		case "Ord":
			if r.Entities[ent].File == 0 && r.hasOrd(t) {
				return true, ""
			}
		}
		res := r.conforms(t, c.Type, from)
		return res.ok, res.missing
	case CFn, CFnMut, CFnOnce:
		n := r.Types.Node(t)
		if n.Kind == KFn {
			if r.callableMatches(t, c.Type) {
				return true, ""
			}
			return false, "its signature is `" + r.TypeString(t) + "`, not `" + r.TypeString(c.Type) + "`"
		}
		if n.Kind == KClosure {
			return true, ""
		}
		return false, "it is not callable"
	}
	return true, ""
}

func (r *Result) callableMatches(fn, want TypeID) bool {
	a, b := r.Types.Node(fn), r.Types.Node(want)
	return sameArgs(a.Args, b.Args) && a.Elem == b.Elem && Effects(a.Flags)&Effects(b.Flags)&(EffPure|EffNoalloc) == Effects(b.Flags)&(EffPure|EffNoalloc)
}
