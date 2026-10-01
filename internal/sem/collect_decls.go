package sem

import (
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// collect (pass 1) creates entities for every top-level declaration and fills
// package scopes; signatures and bodies resolve later.
func (r *Result) collect() {
	for id := 1; id < len(r.Packages); id++ {
		for _, f := range r.Packages[id].Files {
			r.collectFile(f)
		}
	}
	for id := 1; id < len(r.Packages); id++ {
		r.collectEntry(PackageID(id))
	}
	if pre, ok := r.pathIndex["std/prelude"]; ok {
		r.fillPrelude(pre)
	}
}

// fillPrelude makes the pub declarations of std/prelude visible in every
// file without an import.
func (r *Result) fillPrelude(pkg PackageID) {
	for name, b := range r.Scopes[r.Packages[pkg].Scope].Names {
		vis := Visibility{Level: VisPub}
		if b.Ent != 0 {
			vis = r.Entities[b.Ent].Vis
		} else if len(r.Overloads[b.Set].Members) > 0 {
			vis = r.Entities[r.Overloads[b.Set].Members[0]].Vis
		}
		if vis.Level == VisPub {
			r.Scopes[r.prelude].Names[name] = b
		}
	}
}

func (r *Result) collectFile(f FileID) {
	t := r.tree(f)
	for _, d := range r.topDecls(f) {
		switch t.Kind(d) {
		case syntax.FnDecl:
			r.collectFn(f, d, "")
		case syntax.TypeDecl:
			r.collectType(f, d, false)
		case syntax.InterfaceDecl:
			s := ifaceDecl(t, d)
			if uni := r.lookupUniverse(t.TokText(s.Name)); uni != 0 && r.Packages[r.packageOf(f)].Std && r.Entities[uni].Kind == EntInterface {
				r.adoptUniverseIface(f, d, uni)
				continue
			}
			id := r.declareTop(f, d, s.Name, EntInterface, r.resolveVis(f, s.Vis, r.packageOf(f)))
			if id != 0 {
				r.Entities[id].Detail = r.addIface(InterfaceInfo{})
			}
		case syntax.ConstDecl:
			s := constDecl(t, d)
			id := r.declareTop(f, d, s.Name, EntConst, r.resolveVis(f, s.Vis, r.packageOf(f)))
			if id != 0 {
				r.Entities[id].Detail = r.addConst(ConstInfo{})
			}
		case syntax.TestDecl:
			id := r.newEntity(Entity{Kind: EntTest, Name: t.TokText(t.Nodes[d].Lhs), Pkg: r.packageOf(f), File: f, Node: d, Tok: t.Nodes[d].Tok})
			r.Entities[id].Detail = r.addFn(FnInfo{Body: syntax.NodeID(t.Nodes[d].Rhs)})
			r.Files[f].Defs[d] = id
		case syntax.AbiBlock:
			r.collectAbi(f, d)
		}
	}
}

func (r *Result) collectAbi(f FileID, block syntax.NodeID) {
	t := r.tree(f)
	abiArgs := t.Children(syntax.NodeID(t.Nodes[block].Lhs))
	abi := t.TokText(t.Nodes[abiArgs[0]].Tok)
	export := t.Toks[t.Nodes[block].Tok].Kind != token.KwExtern
	noalloc, naked := false, false
	for _, a := range abiArgs[1:] {
		switch t.TokText(t.Nodes[a].Tok) {
		case "noalloc":
			noalloc = true
		case "naked":
			naked = true
			if !export {
				r.errAt(f, a, cNakedExtern)
			}
		}
	}
	for _, d := range t.Children(syntax.NodeID(t.Nodes[block].Rhs)) {
		switch t.Kind(d) {
		case syntax.FnDecl:
			if export {
				r.collectFn(f, d, "")
				if id := r.Files[f].Defs[d]; id != 0 {
					r.Fn(id).Export = abi
					r.Fn(id).Naked = naked
				}
				continue
			}
			r.collectFn(f, d, abi)
			if id := r.Files[f].Defs[d]; id != 0 && noalloc {
				r.Fn(id).Declared |= EffNoalloc
			}
		case syntax.TypeDecl:
			r.collectType(f, d, true)
		}
	}
}

func (r *Result) collectFn(f FileID, d syntax.NodeID, abi string) {
	t := r.tree(f)
	s := fnDecl(t, d)
	pkg := r.packageOf(f)
	name := t.TokText(s.Name)
	e := Entity{Kind: EntFn, Name: name, Pkg: pkg, File: f, Node: d, Tok: s.Name, Vis: r.resolveVis(f, s.Vis, pkg)}
	if t.Toks[s.Name].Kind != token.Ident {
		e.Flags |= EfOperator
	}
	if r.Packages[pkg].Std {
		e.Flags |= EfStd
	}
	info := FnInfo{Body: s.Body, Declared: Effects(s.Mods), Abi: abi}
	if s.Recv != 0 {
		id := r.newEntity(e)
		r.Entities[id].Detail = r.addFn(info)
		r.Files[f].Defs[d] = id
		return
	}
	if !r.checkTopName(f, d, name) {
		return
	}
	id := r.newEntity(e)
	r.Entities[id].Detail = r.addFn(info)
	r.Files[f].Defs[d] = id
	scope := r.Packages[pkg].Scope
	if prev, dup := r.define(scope, name, Binding{}); dup {
		if prev.Set == 0 {
			r.errNote(f, d, prev, "`"+name+"` was first declared here", cNameRedeclared, name, r.Packages[pkg].Path)
			r.Entities[id].Flags |= EfPoison
			return
		}
		r.Overloads[prev.Set].Members = append(r.Overloads[prev.Set].Members, id)
		r.Fns[r.Entities[id].Detail].Set = prev.Set
		return
	}
	set := r.addOverloadSet(OverloadSet{Name: name, Pkg: pkg, Members: []EntityID{id}})
	r.Scopes[scope].Names[name] = Binding{Set: set}
	r.Fns[r.Entities[id].Detail].Set = set
}

func (r *Result) collectType(f FileID, d syntax.NodeID, foreign bool) {
	t := r.tree(f)
	s := typeDecl(t, d)
	pkg := r.packageOf(f)
	if uni := r.lookupUniverse(t.TokText(s.Name)); uni != 0 && r.Packages[pkg].Std && r.Entities[uni].Kind == EntType && s.Body == 0 {
		r.Files[f].Defs[d] = uni
		r.define(r.Packages[pkg].Scope, t.TokText(s.Name), Binding{Ent: uni})
		return
	}
	kind := EntType
	info := TypeDeclInfo{Members: map[string]OverloadSetID{}, Ops: map[string]OverloadSetID{}, Home: pkg}
	switch t.Kind(s.Body) {
	case syntax.AliasBody:
		kind = EntAlias
	case syntax.RecordBody:
		info.Form = FormRecord
	case syntax.ResourceBody:
		info.Form = FormResource
	case syntax.AdtBody:
		info.Form = FormAdt
	default:
		info.Form = FormOpaque
		if !r.Packages[pkg].Std && !foreign {
			r.errAt(f, d, cOpaqueOutsideStd, t.TokText(s.Name))
		}
	}
	id := r.declareTop(f, d, s.Name, kind, r.resolveVis(f, s.Vis, pkg))
	if id == 0 {
		return
	}
	if kind == EntType {
		r.Entities[id].Detail = r.addTypeDecl(info)
	}
	if r.Packages[pkg].Std {
		r.Entities[id].Flags |= EfStd
	}
}
