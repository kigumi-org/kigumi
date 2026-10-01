package sem

// reservedOperator lists comparison operators a user type cannot define,
// since `==` and ordering still compare structurally.
var reservedOperator = map[string]string{"==": "Eq", "!=": "Eq", "<": "Ord", "<=": "Ord", ">": "Ord", ">=": "Ord"}

// checkOperatorDecl validates an operator definition.
func (r *Result) checkOperatorDecl(id EntityID) {
	e := &r.Entities[id]
	info := r.Fn(id)
	op := e.Name
	if proto := reservedOperator[op]; proto != "" && e.Flags&EfStd == 0 {
		r.errAt(e.File, e.Node, cUserOperatorReserved, r.Entities[info.Owner].Name, op, proto)
		return
	}
	if info.SelfParam != 0 {
		r.errAt(e.File, e.Node, cOperatorHasSelf)
		return
	}
	want := 2
	if op[0] == '!' || op[0] == '~' {
		want = 1
	}
	if len(info.Params) != want {
		r.errAt(e.File, e.Node, cOperatorArity, op, want, plural(want))
		return
	}
	owner := info.Owner
	if r.Entities[owner].File == 0 {
		r.errAt(e.File, e.Node, cOperatorPrimitive)
		return
	}
	ownerType := r.ownerInstance(owner)
	for _, p := range info.Params {
		if r.Entities[p].Type == ownerType {
			return
		}
	}
	r.errAt(e.File, e.Node, cOperatorForeignOperand, op, r.Entities[owner].Name)
}

// ownerInstance is the type of `self` in members of owner.
func (r *Result) ownerInstance(owner EntityID) TypeID {
	e := &r.Entities[owner]
	info := r.typeDecl(owner)
	if e.Type != 0 && len(info.Params) == 0 {
		return e.Type
	}
	var args []TypeID
	for _, p := range info.Params {
		args = append(args, r.Types.Param(p))
	}
	return r.Types.Named(owner, args)
}
