package sem

import "kigumi/internal/syntax"

const (
	aliasUnknown uint8 = iota
	aliasComputing
	aliasDone
)

// aliasType expands a transparent alias, detecting cycles. Predeclared
// aliases already carry their target.
func (r *Result) aliasType(id EntityID) TypeID {
	e := &r.Entities[id]
	if e.File == 0 {
		return e.Type
	}
	switch r.aliasState[id] {
	case aliasDone:
		return e.Type
	case aliasComputing:
		r.errAt(e.File, e.Node, cAliasCycle, e.Name)
		e.Type = TyPoison
		e.Flags |= EfPoison
		r.aliasState[id] = aliasDone
		return TyPoison
	}
	r.aliasState[id] = aliasComputing
	t := r.tree(e.File)
	body := typeDecl(t, e.Node).Body
	target := syntax.NodeID(t.Nodes[body].Lhs)
	typ := r.resolveType(e.File, r.declScope(id), target, posAlias)
	if r.aliasState[id] == aliasComputing {
		r.aliasState[id] = aliasDone
		r.Entities[id].Type = typ
	}
	return r.Entities[id].Type
}

// checkFiniteSize rejects records and ADTs that contain themselves by value.
// Containers with indirection break the chain.
func (r *Result) checkFiniteSize(id EntityID) {
	info := r.typeDecl(id)
	if info.Size != sizeUnknown {
		return
	}
	info.Size = sizeComputing
	infinite := false
	for _, fld := range info.Fields {
		if r.containsByValue(r.Entities[fld].Type, id) {
			infinite = true
		}
	}
	for _, v := range info.Variants {
		for _, p := range r.variant(v).Payload {
			if r.containsByValue(p, id) {
				infinite = true
			}
		}
	}
	info = r.typeDecl(id)
	if infinite {
		info.Size = sizeInfinite
		e := &r.Entities[id]
		r.errAt(e.File, e.Node, cRecursiveType, e.Name)
		return
	}
	info.Size = sizeFinite
}

// containsByValue reports whether t stores target inline, walking through
// records and Option/Result but not heap containers or ADT payloads: every
// ADT instance is boxed uniformly, so a field or payload of ADT type is
// already an indirection regardless of what it contains.
func (r *Result) containsByValue(t TypeID, target EntityID) bool {
	n := r.Types.Node(t)
	if n.Kind != KNamed {
		return false
	}
	if n.Ent == r.Types.optionEnt || n.Ent == r.Types.resultEnt {
		for _, a := range n.Args {
			if r.containsByValue(a, target) {
				return true
			}
		}
		return false
	}
	e := &r.Entities[n.Ent]
	if e.File == 0 {
		return n.Ent == target
	}
	info := r.typeDecl(n.Ent)
	if info.Form == FormOpaque || info.Form == FormAdt {
		return false
	}
	if n.Ent == target {
		return true
	}
	if info.Size == sizeComputing {
		return true
	}
	if info.Size == sizeUnknown {
		r.checkFiniteSize(n.Ent)
	}
	// An infinite type was already reported at its declaration; walking into it again would not terminate.
	if r.typeDecl(n.Ent).Size == sizeInfinite {
		return false
	}
	subst := map[EntityID]TypeID{}
	for i, p := range info.Params {
		if i < len(n.Args) {
			subst[p] = n.Args[i]
		}
	}
	for _, fld := range info.Fields {
		if r.containsByValue(r.Types.Subst(r.Entities[fld].Type, subst), target) {
			return true
		}
	}
	return false
}
