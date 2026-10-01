package sem

import "kigumi/internal/syntax"

func (r *Result) declareIfaceBinder(id EntityID) {
	e := &r.Entities[id]
	t := r.tree(e.File)
	s := ifaceDecl(t, e.Node)
	scope := r.declScope(id)
	info := r.iface(id)
	info.Params = r.declareGenerics(e.File, id, s.Generics, scope)
	self := r.newEntity(Entity{Kind: EntTypeParam, Name: "Self", Pkg: e.Pkg, File: e.File, Node: e.Node, Parent: id, Vis: Visibility{Level: VisPub}})
	r.Entities[self].Detail = r.addTypeParam(TypeParamInfo{Index: -1})
	r.Entities[self].Type = r.Types.Param(self)
	r.define(scope, "Self", Binding{Ent: self})
	r.iface(id).SelfParam = self
}

func (r *Result) declareIfaceConstraints(id EntityID) {
	e := &r.Entities[id]
	s := ifaceDecl(r.tree(e.File), e.Node)
	r.resolveConstraints(e.File, r.declScope(id), s.Generics, r.iface(id).Params)
}

// declareRequirements: a requirement without a receiver is a static
// requirement, satisfied by an associated function (`fn T.fromJson(v)`).
func (r *Result) declareRequirements(id EntityID) {
	e := &r.Entities[id]
	f := e.File
	t := r.tree(f)
	s := ifaceDecl(t, e.Node)
	scope := r.declScope(id)
	for _, m := range t.Children(s.Members) {
		if t.Kind(m) != syntax.FnDecl {
			r.errAt(f, m, cIfaceMemberKind)
			continue
		}
		fs := fnDecl(t, m)
		name := t.TokText(fs.Name)
		req := r.newEntity(Entity{Kind: EntFn, Name: name, Pkg: e.Pkg, File: f, Node: m, Tok: fs.Name, Parent: id, Vis: e.Vis})
		r.Entities[req].Detail = r.addFn(FnInfo{Declared: Effects(fs.Mods), Owner: id, EffectPoly: fs.Mods&syntax.ModPureVar != 0})
		r.Files[f].Defs[m] = req
		if fs.Body != 0 {
			r.errAt(f, fs.Body, cIfaceDefaultBody)
		}
		if fs.Vis != 0 {
			r.errAt(f, fs.Vis, cVisRedundantPub)
		}
		if fs.Mods&syntax.ModAsync != 0 {
			r.errAt(f, m, cIfaceAsyncReq)
		}
		if fs.Recv != 0 {
			r.errAt(f, fs.Recv, cIfaceMemberKind)
		}
		reqScope := r.newScope(ScopeFn, scope, f, m)
		r.Scopes[reqScope].Fn = req
		r.declScopes[req] = reqScope
		own := r.declareGenerics(f, req, fs.Generics, reqScope)
		r.resolveConstraints(f, reqScope, fs.Generics, own)
		r.Fn(req).TypeParams = own
		r.resolveSignature(req, reqScope, r.Types.Param(r.iface(id).SelfParam))
		r.iface(id).Reqs = append(r.iface(id).Reqs, req)
	}
}

// objectSafe decides lazily whether an interface can be a value type.
func (r *Result) objectSafe(id EntityID) bool {
	info := r.iface(id)
	switch info.ObjectSafe {
	case 1:
		return true
	case -1:
		return false
	}
	info.ObjectSafe = 1
	self := r.Types.Param(info.SelfParam)
	for _, req := range info.Reqs {
		fi := r.Fn(req)
		name := r.Entities[req].Name
		if len(fi.TypeParams) > 0 {
			r.iface(id).ObjectSafe, r.iface(id).ObjectSafeWhy = -1, "requirement `"+name+"` is generic"
			return false
		}
		if fi.Recv == RecvNone {
			r.iface(id).ObjectSafe, r.iface(id).ObjectSafeWhy = -1, "requirement `"+name+"` has no receiver"
			return false
		}
		if fi.Sig != 0 && r.Types.Contains(fi.Sig, self) {
			r.iface(id).ObjectSafe, r.iface(id).ObjectSafeWhy = -1, "`Self` appears outside the receiver of `"+name+"`"
			return false
		}
	}
	return true
}

func (r *Result) requirement(id EntityID, name string) EntityID {
	for _, req := range r.iface(id).Reqs {
		if r.Entities[req].Name == name {
			return req
		}
	}
	return 0
}
