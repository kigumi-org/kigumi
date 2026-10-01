package sem

import (
	"slices"

	"kigumi/internal/syntax"
)

// adoptUniverseIface lets a std stub supply the requirements of a predeclared
// interface (e.g. `Error`).
func (r *Result) adoptUniverseIface(f FileID, d syntax.NodeID, uni EntityID) {
	t := r.tree(f)
	s := ifaceDecl(t, d)
	pkg := r.packageOf(f)
	e := &r.Entities[uni]
	e.File, e.Node, e.Tok, e.Pkg = f, d, s.Name, pkg
	r.Files[f].Defs[d] = uni
	r.define(r.Packages[pkg].Scope, e.Name, Binding{Ent: uni})
}

func (r *Result) declareTop(f FileID, d syntax.NodeID, nameTok uint32, kind EntityKind, vis Visibility) EntityID {
	t := r.tree(f)
	name := t.TokText(nameTok)
	if !r.checkTopName(f, d, name) {
		return 0
	}
	pkg := r.packageOf(f)
	id := r.newEntity(Entity{Kind: kind, Name: name, Pkg: pkg, File: f, Node: d, Tok: nameTok, Vis: vis})
	if r.Packages[pkg].Std {
		r.Entities[id].Flags |= EfStd
	}
	r.Files[f].Defs[d] = id
	if prev, dup := r.define(r.Packages[pkg].Scope, name, Binding{Ent: id}); dup {
		r.errNote(f, d, prev, "`"+name+"` was first declared here", cNameRedeclared, name, r.Packages[pkg].Path)
		r.Entities[id].Flags |= EfPoison
	}
	return id
}

// checkTopName rejects `nil`, `panic`, predeclared names (PRE-3, PRE-4), and
// anywhere in the entry package the entry-only intrinsics entryScope binds:
// that scope reaches every file of the package via the scope chain, not just
// the entry file, so the check must too.
func (r *Result) checkTopName(f FileID, d syntax.NodeID, name string) bool {
	pkg := r.packageOf(f)
	switch {
	case name == "nil":
		r.errAt(f, d, cNilName)
	case name == "panic":
		r.errAt(f, d, cPanicDeclared)
	case r.lookupUniverse(name) != 0 && !r.Packages[pkg].Std:
		r.errAt(f, d, cPredeclaredRedeclared, name)
	case r.Packages[pkg].Entry != 0 && slices.Contains(r.entryOnlyNames(), name):
		r.errAt(f, d, cEntryNameRedeclared, name)
	default:
		return true
	}
	return false
}
