package sem

import "strings"

// methodOwnParams is the suffix of m's type parameters past the owner type's
// own: what a call to m still has to supply once the receiver instance
// is known.
func (r *Result) methodOwnParams(m EntityID) []EntityID {
	info := r.Fn(m)
	ownerParams := r.ownerParams(m)
	if len(info.TypeParams) <= len(ownerParams) {
		return nil
	}
	return info.TypeParams[len(ownerParams):]
}

// unifyMethodOwn tries to determine own from got against want,
// structurally, one pass with no backtracking; it returns the parameters
// that stayed undetermined.
func (r *Result) unifyMethodOwn(own []EntityID, got, want typeNode) (map[EntityID]TypeID, []EntityID) {
	isOwn := make(map[EntityID]bool, len(own))
	for _, p := range own {
		isOwn[p] = true
	}
	subst := map[EntityID]TypeID{}
	ok := len(got.Args) == len(want.Args) && r.unifyOwnType(isOwn, got.Elem, want.Elem, subst)
	for i := 0; ok && i < len(got.Args); i++ {
		ok = r.unifyOwnType(isOwn, got.Args[i], want.Args[i], subst)
	}
	var undetermined []EntityID
	for _, p := range own {
		if _, has := subst[p]; !ok || !has {
			undetermined = append(undetermined, p)
		}
	}
	return subst, undetermined
}

// unifyOwnType matches one of own's parameters, wherever it sits inside got,
// against the closed type want carries there; every other position must
// already agree structurally.
func (r *Result) unifyOwnType(isOwn map[EntityID]bool, got, want TypeID, subst map[EntityID]TypeID) bool {
	if got == want {
		return true
	}
	tt := r.Types
	gn := tt.Node(got)
	if gn.Kind == KParam && isOwn[gn.Ent] {
		if prev, bound := subst[gn.Ent]; bound {
			return prev == want
		}
		subst[gn.Ent] = want
		return true
	}
	wn := tt.Node(want)
	if gn.Kind != wn.Kind || gn.Flags != wn.Flags || gn.Ent != wn.Ent || gn.Var != wn.Var || len(gn.Args) != len(wn.Args) {
		return false
	}
	if !r.unifyOwnType(isOwn, gn.Elem, wn.Elem, subst) {
		return false
	}
	for i := range gn.Args {
		if !r.unifyOwnType(isOwn, gn.Args[i], wn.Args[i], subst) {
			return false
		}
	}
	return true
}

// ownInstArgs turns findWitness's substitution into positional
// instantiation arguments, declaration order, for deferWitnesses/
// witnessSubst (nil when m has none).
func (r *Result) ownInstArgs(m EntityID, ownSubst map[EntityID]TypeID) []TypeID {
	own := r.methodOwnParams(m)
	if len(own) == 0 {
		return nil
	}
	out := make([]TypeID, len(own))
	for i, p := range own {
		out[i] = ownSubst[p]
	}
	return out
}

// genericMethodPrefix names m's own type parameters and the requirement it
// was tried against, for findWitness's diagnostics.
func (r *Result) genericMethodPrefix(m, req EntityID, own []EntityID) string {
	names := make([]string, len(own))
	for i, p := range own {
		names[i] = r.Entities[p].Name
	}
	sig := r.Entities[m].Name + "[" + strings.Join(names, ", ") + "]"
	iface := r.Entities[r.Entities[req].Parent].Name
	return "generic method `" + sig + "` cannot satisfy `" + iface + "." + r.Entities[req].Name + "`: "
}

// genericMethodWhy names the undetermined parameters unifyMethodOwn left
// behind, for findWitness's diagnostic.
func (r *Result) genericMethodWhy(m, req EntityID, own, undetermined []EntityID) string {
	missing := make([]string, len(undetermined))
	for i, p := range undetermined {
		missing[i] = "`" + r.Entities[p].Name + "`"
	}
	verb := "is"
	if len(missing) > 1 {
		verb = "are"
	}
	return r.genericMethodPrefix(m, req, own) + strings.Join(missing, ", ") + " " + verb + " not determined by the requirement"
}

// ownConstraintWhy checks an own parameter's determination against its
// own declared constraints: unifying determines a type but doesn't prove it
// legal there, e.g. an own parameter bare where the requirement takes
// `&Self` (Eq/Ord's `other: &Self`) determines a reference, which an
// ordinary interface constraint essentially never satisfies.
func (r *Result) ownConstraintWhy(m, req EntityID, own []EntityID, candOwn map[EntityID]TypeID, from PackageID) string {
	for _, p := range own {
		t := candOwn[p]
		for _, con := range r.typeParam(p).Constraints {
			if ok, why := r.satisfies(t, con, from); !ok {
				return r.genericMethodPrefix(m, req, own) + "`" + r.Entities[p].Name + "` is determined as `" +
					r.TypeString(t) + "`, which does not satisfy its own constraint: " + why
			}
		}
	}
	return ""
}
