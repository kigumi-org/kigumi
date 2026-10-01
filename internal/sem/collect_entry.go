package sem

import "kigumi/internal/syntax"

// collectEntry classifies top-level statements: the entry file's
// statements form the implicit main; anywhere else they are errors.
func (r *Result) collectEntry(pkg PackageID) {
	p := &r.Packages[pkg]
	for _, f := range p.Files {
		t := r.tree(f)
		var stmts []syntax.NodeID
		for _, d := range r.topDecls(f) {
			if isStatementKind(t.Kind(d)) {
				stmts = append(stmts, d)
			}
		}
		if f != p.Entry {
			for _, s := range stmts {
				r.errAt(f, s, cEntryStmtOutsideEntry)
			}
			continue
		}
		r.entryScope(f)
		if len(stmts) == 0 {
			continue
		}
		p.HasScript = true
		id := r.newEntity(Entity{Kind: EntImplicitMain, Name: "main", Pkg: pkg, File: f, Node: t.Root, Tok: 0})
		r.Entities[id].Detail = r.addFn(FnInfo{Body: t.Root})
		r.implicitMain[pkg] = id
		if b, ok := r.Scopes[p.Scope].Names["main"]; ok && b.Set != 0 {
			for _, m := range r.Overloads[b.Set].Members {
				r.errAt(r.Entities[m].File, r.Entities[m].Node, cMainImplicitExplicit)
			}
		}
	}
}

// entryScope adds the entry-only names host, print and eprint under the
// file scope of an entry file. Freestanding targets have no
// host capabilities; print still reaches the embedder's rt_sys_write.
func (r *Result) entryScope(f FileID) {
	scope := r.newScope(ScopeEntry, r.fileScopes[f], f, r.tree(f).Root)
	r.fileScopes[f] = scope
	r.Files[f].Scopes[r.tree(f).Root] = scope
	r.Files[f].Scopes[r.tree(f).Root] = scope
	for _, name := range r.entryOnlyNames() {
		id := r.newEntity(Entity{Kind: EntIntrinsic, Name: name, File: f, Flags: EfEntryOnly, Vis: Visibility{Level: VisPub}})
		r.define(scope, name, Binding{Ent: id})
	}
}

// entryOnlyNames lists the names entryScope binds in an entry file; checkTopName
// uses the same list to reject a top-level declaration of one of them before
// entryScope runs and silently shadows it.
func (r *Result) entryOnlyNames() []string {
	names := []string{"host", "print", "eprint"}
	if r.target == Freestanding {
		names = names[1:]
	}
	return names
}
