package sem

func (tt *TypeTable) Fn(params []TypeID, ret TypeID, eff Effects, variadic bool) TypeID {
	flags := uint16(eff)
	if variadic {
		flags |= fnVariadic
	}
	return tt.Intern(typeNode{Kind: KFn, Flags: flags, Elem: ret, Args: params})
}

// FnEffectPoly builds a `pure? fn(A) -> B` effect-polymorphic type; EffPure in eff is ignored.
func (tt *TypeTable) FnEffectPoly(params []TypeID, ret TypeID, eff Effects, variadic bool) TypeID {
	flags := uint16(eff&^EffPure) | fnEffectPoly
	if variadic {
		flags |= fnVariadic
	}
	return tt.Intern(typeNode{Kind: KFn, Flags: flags, Elem: ret, Args: params})
}

func (tt *TypeTable) FnCPtr(params []TypeID, ret TypeID, variadic bool) TypeID {
	flags := fnCAbi
	if variadic {
		flags |= fnCVariadic
	}
	return tt.Intern(typeNode{Kind: KFn, Flags: flags, Elem: ret, Args: params})
}

// IsCFn reports whether t is a C function pointer type.
func (tt *TypeTable) IsCFn(t TypeID) bool {
	n := tt.Node(t)
	return n.Kind == KFn && n.Flags&fnCAbi != 0
}

func (tt *TypeTable) Ref(t TypeID, mut bool) TypeID {
	return tt.Intern(typeNode{Kind: KRef, Flags: mutFlag(mut), Elem: t})
}

// RefMut reports whether t is `&mut U` (false for a shared `&U` or a
// non-Ref type).
func (tt *TypeTable) RefMut(t TypeID) bool {
	n := tt.Node(t)
	return n.Kind == KRef && n.Flags&flagMut != 0
}

func (tt *TypeTable) Ptr(t TypeID, mut bool) TypeID {
	return tt.Intern(typeNode{Kind: KPtr, Flags: mutFlag(mut), Elem: t})
}

func mutFlag(mut bool) uint16 {
	if mut {
		return flagMut
	}
	return 0
}
