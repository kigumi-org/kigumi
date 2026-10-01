package sem

// memberSet returns the overload set of member name on type t, or 0.
func (r *Result) memberSet(t TypeID, name string) OverloadSetID {
	n := r.Types.Node(t)
	var owner EntityID
	switch n.Kind {
	case KNamed:
		owner = n.Ent
	case KPrim:
		owner = r.primEntity[t]
	default:
		return 0
	}
	if owner == 0 {
		return 0
	}
	return r.typeDecl(owner).Members[name]
}

// memberSig is the signature of member m as seen on the instance t, with
// the owner's type parameters substituted.
func (r *Result) memberSig(t TypeID, m EntityID) TypeID {
	n := r.Types.Node(t)
	if n.Kind != KNamed {
		return r.Fn(m).Sig
	}
	params := r.typeDecl(n.Ent).Params
	if len(params) == 0 {
		return r.Fn(m).Sig
	}
	subst := map[EntityID]TypeID{}
	for i, p := range params {
		if i < len(n.Args) {
			subst[p] = n.Args[i]
		}
	}
	for i, p := range r.Fn(m).RecvParams {
		if i < len(n.Args) {
			subst[p] = n.Args[i]
		}
	}
	return r.Types.Subst(r.Fn(m).Sig, subst)
}

func (r *Result) ownerParams(m EntityID) []EntityID {
	if rp := r.Fn(m).RecvParams; len(rp) > 0 {
		return rp
	}
	if owner := r.Fn(m).Owner; owner != 0 && r.Entities[owner].Kind == EntType {
		return r.typeDecl(owner).Params
	}
	return nil
}
