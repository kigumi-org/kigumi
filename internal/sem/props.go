package sem

// derives Copy from a type's structure; memoized in props.
func (r *Result) isCopy(t TypeID) bool {
	if t == 0 {
		return true
	}
	p := &r.Types.props[t]
	switch p.copy {
	case 1:
		return true
	case -1:
		return false
	}
	p.copy = 1
	res := r.computeCopy(t)
	if !res {
		r.Types.props[t].copy = -1
	}
	return res
}

func (r *Result) computeCopy(t TypeID) bool {
	tt := r.Types
	n := tt.Node(t)
	switch n.Kind {
	case KPoison, KPrim, KUntyped, KFn, KPtr:
		return true
	case KRef:
		return n.Flags&flagMut == 0
	case KIface, KVar:
		return false
	case KClosure:
		if n.Ent == 0 {
			return false
		}
		return r.closure(n.Ent).Copy
	case KParam:
		for _, c := range r.typeParam(n.Ent).Constraints {
			if c.Kind == CCopy {
				return true
			}
		}
		return false
	case KNamed:
		return r.namedCopy(n)
	}
	return false
}

// ScalarRefElem returns (T, true) when t is `&mut T` and T is a scalar kind that can be written through.
// String and Bytes are Copy but excluded: their runtime form isn't a plain scalar, so writing through needs a move.
func (r *Result) ScalarRefElem(t TypeID) (TypeID, bool) {
	tt := r.Types
	if tt.Kind(t) != KRef || tt.Node(t).Flags&flagMut == 0 {
		return 0, false
	}
	elem := tt.Node(t).Elem
	if tt.Kind(elem) != KPrim {
		return 0, false
	}
	switch elem {
	case TyUnit, TyNever, TyString, TyBytes:
		return 0, false
	}
	return elem, true
}

func (r *Result) namedCopy(n typeNode) bool {
	e := &r.Entities[n.Ent]
	info := r.typeDecl(n.Ent)
	// FixedArray[T, N] wraps an Array[T], so the field walk below would wrongly say
	// "not Copy"; FixedArray's value semantics depend only on T.
	if n.Ent == r.langItem(0, 0, "FixedArray") && len(n.Args) > 0 {
		return r.isCopy(n.Args[0])
	}
	if info.Form == FormResource {
		return false
	}
	if info.Form == FormOpaque {
		// generic opaque types (Array, Map, Shared) own heap storage and move; non-generic ones
		// are handles and copy freely.
		return len(info.Params) == 0
	}
	subst := map[EntityID]TypeID{}
	for i, p := range info.Params {
		if i < len(n.Args) {
			subst[p] = n.Args[i]
		}
	}
	for _, fld := range info.Fields {
		if !r.isCopy(r.Types.Subst(r.Entities[fld].Type, subst)) {
			return false
		}
	}
	for _, v := range info.Variants {
		for _, p := range r.variant(v).Payload {
			if !r.isCopy(r.Types.Subst(p, subst)) {
				return false
			}
		}
	}
	_ = e
	return true
}
