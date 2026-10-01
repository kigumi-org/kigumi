package sem

func (tt *TypeTable) Named(ent EntityID, args []TypeID) TypeID {
	return tt.Intern(typeNode{Kind: KNamed, Ent: ent, Args: args})
}

func (tt *TypeTable) Option(t TypeID) TypeID { return tt.Named(tt.optionEnt, []TypeID{t}) }

func (tt *TypeTable) Result(t, e TypeID) TypeID { return tt.Named(tt.resultEnt, []TypeID{t, e}) }

func (tt *TypeTable) Iface(ent EntityID, args []TypeID) TypeID {
	return tt.Intern(typeNode{Kind: KIface, Ent: ent, Args: args})
}

func (tt *TypeTable) Param(ent EntityID) TypeID {
	return tt.Intern(typeNode{Kind: KParam, Ent: ent})
}

func (tt *TypeTable) Closure(ent EntityID) TypeID {
	return tt.Intern(typeNode{Kind: KClosure, Ent: ent})
}

func (tt *TypeTable) Var(n uint32) TypeID { return tt.Intern(typeNode{Kind: KVar, Var: n}) }

// ConstVal interns a const generic argument value: ty is its
// declared type (usize, a fixed-width integer, Int or Bool).
func (tt *TypeTable) ConstVal(v int64, ty TypeID) TypeID {
	idx, ok := tt.constIndex[v]
	if !ok {
		idx = uint32(len(tt.constVals))
		tt.constVals = append(tt.constVals, v)
		tt.constIndex[v] = idx
	}
	return tt.Intern(typeNode{Kind: KConst, Var: idx, Elem: ty})
}

// ConstValue reads back a KConst type's value and declared type.
func (tt *TypeTable) ConstValue(t TypeID) (int64, TypeID) {
	n := tt.nodes[t]
	return tt.constVals[n.Var], n.Elem
}

// IsConst reports whether t is a const generic argument value.
func (tt *TypeTable) IsConst(t TypeID) bool { return t != 0 && tt.nodes[t].Kind == KConst }
