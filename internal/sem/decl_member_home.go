package sem

import "kigumi/internal/syntax"

// homeOf is the package allowed to define members of owner.
func (r *Result) homeOf(owner EntityID) PackageID {
	if r.Entities[owner].File != 0 {
		return r.Entities[owner].Pkg
	}
	if id, ok := r.pathIndex[r.homePath(owner)]; ok {
		return id
	}
	return 0
}

func (r *Result) homePath(owner EntityID) string {
	if r.Entities[owner].File != 0 {
		return r.Packages[r.Entities[owner].Pkg].Path
	}
	if owner == r.Types.arrayEnt {
		return "std/array"
	}
	return "std/prelude"
}

func (r *Result) paramNames(params []EntityID) string {
	out := ""
	for i, p := range params {
		if i > 0 {
			out += ", "
		}
		out += r.Entities[p].Name
	}
	return out
}

// registerMember puts a method, associated function or operator into its
// owner's member tables and checks the drop signature.
func (r *Result) registerMember(id EntityID) {
	e := &r.Entities[id]
	info := r.Fn(id)
	owner := info.Owner
	tinfo := r.typeDecl(owner)
	name := e.Name
	if e.Flags&EfOperator != 0 {
		r.checkOperatorDecl(id)
		r.addMember(tinfo.Ops, name, id, owner)
		return
	}
	if r.findField(owner, name) != 0 {
		r.errAt(e.File, e.Node, cFieldMethodClash, name, r.Entities[owner].Name)
	}
	if name == "drop" {
		r.checkDrop(id, owner)
	}
	r.addMember(tinfo.Members, name, id, owner)
}

func (r *Result) addMember(table map[string]OverloadSetID, name string, id, owner EntityID) {
	set, ok := table[name]
	if !ok {
		set = r.addOverloadSet(OverloadSet{Name: name, Pkg: r.Entities[id].Pkg, Owner: owner})
		table[name] = set
	}
	r.Overloads[set].Members = append(r.Overloads[set].Members, id)
	r.Fn(id).Set = set
}

func (r *Result) findField(owner EntityID, name string) EntityID {
	for _, fld := range r.typeDecl(owner).Fields {
		if r.Entities[fld].Name == name {
			return fld
		}
	}
	return 0
}

func (r *Result) checkDrop(id, owner EntityID) {
	e := &r.Entities[id]
	info := r.Fn(id)
	tinfo := r.typeDecl(owner)
	ownerName := r.Entities[owner].Name
	switch {
	case tinfo.Form != FormResource:
		r.errAt(e.File, e.Node, cDropOnNonResource)
	case info.Recv != RecvMove || len(info.Params) != 0 || r.Types.Node(info.Sig).Elem != TyUnit || info.Declared&EffAsync != 0:
		r.errAt(e.File, e.Node, cDropSignature, ownerName)
	case tinfo.Drop != 0:
		r.errAt(e.File, e.Node, cDropDuplicate, ownerName)
	default:
		tinfo.Drop = id
	}
}

// authorizeForeignMember checks the orphan rule: some local
// interface requirement named e.Name must match id's signature with Self
// standing for ownerType. It disowns id and reports an error when none
// does, naming any same-named but differently-shaped requirement in a note.
func (r *Result) authorizeForeignMember(id, owner EntityID, ownerType TypeID, recv syntax.NodeID) {
	e := &r.Entities[id]
	f := e.File
	if r.homeOf(owner) == r.packageOf(f) {
		return
	}
	var mismatch EntityID
	for iid := 1; iid < len(r.Entities); iid++ {
		ie := &r.Entities[iid]
		if ie.Kind != EntInterface || ie.Pkg != r.packageOf(f) {
			continue
		}
		req := r.requirement(EntityID(iid), e.Name)
		if req == 0 {
			continue
		}
		if r.matchesRequirement(id, req, ownerType) {
			return
		}
		if mismatch == 0 {
			mismatch = req
		}
	}
	typeName := r.Entities[owner].Name
	if mismatch == 0 {
		r.errAt(f, recv, cMemberForeignType, typeName, r.homePath(owner))
	} else {
		sig := r.reqText(e.Name, r.Fn(mismatch).Recv, r.substRequirement(mismatch, ownerType))
		note := "local requirement `" + e.Name + "` needs `" + sig + "`"
		r.errNote(f, recv, Binding{Ent: mismatch}, note, cMemberForeignType, typeName, r.homePath(owner))
	}
	r.Fn(id).Owner = 0
}

// matchesRequirement compares id's signature to req's, with Self
// substituted for ownerType.
func (r *Result) matchesRequirement(id, req EntityID, ownerType TypeID) bool {
	info, rinfo := r.Fn(id), r.Fn(req)
	if info.Recv != rinfo.Recv {
		return false
	}
	want := r.Types.Node(r.substRequirement(req, ownerType))
	got := r.Types.Node(info.Sig)
	return sameArgs(got.Args, want.Args) && got.Elem == want.Elem
}

func (r *Result) substRequirement(req EntityID, ownerType TypeID) TypeID {
	rinfo := r.Fn(req)
	self := r.iface(rinfo.Owner).SelfParam
	return r.Types.Subst(rinfo.Sig, map[EntityID]TypeID{self: ownerType})
}
