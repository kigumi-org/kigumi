package sem

// sameRequirementSig matches each requirement's own type parameters by
// position, not entity: two otherwise-identical `U`s get distinct TypeIDs.
func (r *Result) sameRequirementSig(reqA EntityID, sigA TypeID, reqB EntityID, sigB TypeID) bool {
	ownA, ownB := r.methodOwnParams(reqA), r.methodOwnParams(reqB)
	if len(ownA) != len(ownB) {
		return false
	}
	if len(ownA) == 0 {
		return sigA == sigB
	}
	bind := make(map[EntityID]EntityID, len(ownA))
	for i, a := range ownA {
		bind[a] = ownB[i]
	}
	var eq func(a, b TypeID) bool
	eq = func(a, b TypeID) bool {
		if a == b {
			return true
		}
		an, bn := r.Types.Node(a), r.Types.Node(b)
		if an.Kind == KParam {
			if want, isOwn := bind[an.Ent]; isOwn {
				return bn.Kind == KParam && bn.Ent == want
			}
		}
		if an.Kind != bn.Kind || an.Flags != bn.Flags || an.Ent != bn.Ent || an.Var != bn.Var || len(an.Args) != len(bn.Args) {
			return false
		}
		if !eq(an.Elem, bn.Elem) {
			return false
		}
		for i := range an.Args {
			if !eq(an.Args[i], bn.Args[i]) {
				return false
			}
		}
		return true
	}
	for i, a := range ownA {
		if !sameConstraintSet(r.typeParam(a).Constraints, r.typeParam(ownB[i]).Constraints, eq) {
			return false
		}
	}
	return eq(sigA, sigB)
}

// sameConstraintSet checks that two own type parameters bound by
// sameRequirementSig's bijection declare the same constraints, order aside:
// a `U: Show` and an unconstrained `U` must not coalesce.
func sameConstraintSet(a, b []Constraint, eq func(TypeID, TypeID) bool) bool {
	if len(a) != len(b) {
		return false
	}
	used := make([]bool, len(b))
	for _, ca := range a {
		matched := false
		for j, cb := range b {
			if used[j] || ca.Kind != cb.Kind || !eq(ca.Type, cb.Type) {
				continue
			}
			used[j] = true
			matched = true
			break
		}
		if !matched {
			return false
		}
	}
	return true
}
