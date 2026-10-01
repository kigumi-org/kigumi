package sem

import "kigumi/internal/syntax"

func (r *Result) resolveImports() {
	r.stdImportGraph = r.buildStdImportGraph()
	r.importEdges = make([][]PackageID, len(r.Packages))
	for id := 1; id < len(r.Packages); id++ {
		for _, f := range r.Packages[id].Files {
			for _, d := range r.topDecls(f) {
				if r.tree(f).Kind(d) == syntax.ImportDecl {
					r.resolveImport(f, d)
				}
			}
		}
	}
	r.resolveReexports()
	r.buildPackageGraph()
}

func (r *Result) resolveImport(f FileID, d syntax.NodeID) {
	t := r.tree(f)
	s := importDecl(t, d)
	from := r.packageOf(f)
	path := pathText(t, s.Path, "/")
	target, ok := r.pathIndex[path]
	blockCode, offendingPath, platformBlocked := r.platformAvailability(from, path)
	artifactOffender, artifactBlocked := r.artifactAvailability(from, path)
	switch {
	case !ok:
		r.errAt(f, s.Path, cUnresolvedPackage, path)
		return
	case path == "std/prelude":
		r.errAt(f, s.Path, cImportPreludeExplicit)
		return
	case target == from:
		r.errAt(f, s.Path, cImportSelf, path)
		return
	case r.Packages[target].HasScript:
		r.errAt(f, s.Path, cImportEntryPackage, path)
		return
	case platformBlocked:
		r.errAt(f, s.Path, blockCode, offendingPath)
		return
	case artifactBlocked:
		r.errAt(f, s.Path, cArtifactAvailability, artifactOffender, r.artifactName)
		return
	}
	r.importEdges[from] = append(r.importEdges[from], target)
	vis := Visibility{Level: VisPrivate, Scope: from}
	reexport := s.Vis != 0
	if reexport {
		vis = r.resolveVis(f, s.Vis, from)
	}
	if t.Kind(s.Binding) == syntax.List {
		for _, n := range t.Children(s.Binding) {
			if t.Kind(n) == syntax.ImportAlias {
				orig, alias := syntax.NodeID(t.Nodes[n].Lhs), syntax.NodeID(t.Nodes[n].Rhs)
				r.bindImport(f, alias, from, target, t.TokText(t.Nodes[alias].Tok), vis, reexport, true, t.TokText(t.Nodes[orig].Tok), orig)
				continue
			}
			r.bindImport(f, n, from, target, t.TokText(t.Nodes[n].Tok), vis, reexport, true, "", 0)
		}
		return
	}
	r.bindImport(f, s.Binding, from, target, t.TokText(t.Nodes[s.Binding].Tok), vis, reexport, false, "", 0)
}

// origName/origNode name the pre-`as` identifier of a renamed selected
// import; empty/zero for every other import.
func (r *Result) bindImport(f FileID, node syntax.NodeID, from, target PackageID, name string, vis Visibility, reexport, selected bool, origName string, origNode syntax.NodeID) {
	if r.lookupUniverse(name) != 0 || r.lookupPrelude(name) != 0 {
		r.errAt(f, node, cImportPreludeName, name)
		return
	}
	e := Entity{Kind: EntImport, Name: name, Pkg: from, File: f, Node: node, Tok: r.tree(f).Nodes[node].Tok, Vis: vis}
	if reexport {
		e.Flags |= EfReexport
	}
	if !selected {
		e.Target = r.pkgEntity[target]
	}
	id := r.newEntity(e)
	r.Files[f].Defs[node] = id
	if prev, dup := r.define(r.fileScopes[f], name, Binding{Ent: id}); dup {
		r.errNote(f, node, prev, "`"+name+"` was bound here", cImportConflict, name)
		r.Entities[id].Flags |= EfPoison
		return
	}
	if prev, clash := r.Scopes[r.Packages[from].Scope].Names[name]; clash && !reexport {
		r.errNote(f, node, prev, "`"+name+"` is declared here", cImportConflict, name)
		r.Entities[id].Flags |= EfPoison
		return
	}
	switch {
	case selected:
		if origName == "" {
			origName = name
		}
		r.pendingImports = append(r.pendingImports, pendingImport{ent: id, target: target, origName: origName, origNode: origNode})
	case reexport:
		r.publishReexport(id)
	}
}

type pendingImport struct {
	ent    EntityID
	target PackageID
	// name resolveSelected looks up in the target package's scope.
	origName string
	// non-zero only for a renamed import, so hover/go-to-definition on the
	// pre-`as` identifier also resolves.
	origNode syntax.NodeID
}

func (r *Result) resolveSelected(p pendingImport) (done bool) {
	e := &r.Entities[p.ent]
	b, ok := r.Scopes[r.Packages[p.target].Scope].Names[p.origName]
	if !ok || (b.Ent != 0 && r.poisoned(b.Ent)) {
		if !ok {
			node := e.Node
			if p.origNode != 0 {
				node = p.origNode
			}
			r.errAt(e.File, node, cUnresolvedMember, r.Packages[p.target].Path, p.origName)
		}
		e.Flags |= EfPoison
		return true
	}
	if r.hasPendingReexport(p.target, p.origName) {
		return false
	}
	if b.Set != 0 {
		set := &r.Overloads[b.Set]
		e.Target = set.Members[0]
		e.Detail = uint32(b.Set)
		for _, m := range set.Members {
			if !r.useEntity(e.File, e.Node, m) {
				e.Flags |= EfPoison
			}
		}
	} else {
		e.Target = r.resolveAlias(b.Ent)
		if !r.useEntity(e.File, e.Node, e.Target) {
			e.Flags |= EfPoison
		}
	}
	if p.origNode != 0 {
		r.Files[e.File].Uses[p.origNode] = e.Target
	}
	return true
}

func (r *Result) resolveAlias(id EntityID) EntityID {
	for id != 0 && r.Entities[id].Kind == EntImport && r.Entities[id].Target != 0 &&
		r.Entities[r.Entities[id].Target].Kind != EntPackage {
		id = r.Entities[id].Target
	}
	return id
}

func (r *Result) lookupPrelude(name string) EntityID {
	if b, ok := r.Scopes[r.prelude].Names[name]; ok {
		return b.Ent
	}
	return 0
}
