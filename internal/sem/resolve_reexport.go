package sem

import "slices"

// Re-exports may chain across packages, so this iterates to a fixed point;
// leftovers are cycles.
func (r *Result) resolveReexports() {
	pending := r.pendingImports
	r.pendingImports = nil
	for _, p := range pending {
		if r.Entities[p.ent].Flags&EfReexport != 0 {
			r.publishReexport(p.ent)
		}
	}
	for progress := true; progress && len(pending) > 0; {
		progress = false
		var rest []pendingImport
		for _, p := range pending {
			if r.resolveSelected(p) {
				progress = true
				continue
			}
			rest = append(rest, p)
		}
		pending = rest
	}
	for _, p := range pending {
		e := &r.Entities[p.ent]
		r.errAt(e.File, e.Node, cReexportCycle, e.Name)
		e.Flags |= EfPoison
	}
	for id := 1; id < len(r.Entities); id++ {
		e := &r.Entities[id]
		if e.Kind == EntImport && e.Flags&EfReexport != 0 && e.Flags&EfPoison == 0 {
			r.checkReexportWidening(EntityID(id))
		}
	}
}

func (r *Result) hasPendingReexport(pkg PackageID, name string) bool {
	b, ok := r.Scopes[r.Packages[pkg].Scope].Names[name]
	if !ok || b.Ent == 0 {
		return false
	}
	e := &r.Entities[b.Ent]
	return e.Kind == EntImport && e.Target == 0 && e.Flags&EfPoison == 0
}

// Entry-only intrinsics live in a scope spliced above the entry file's own
// scope, not the package scope this defines into, so r.define's same-scope
// dup check can't catch that clash on its own.
func (r *Result) publishReexport(id EntityID) {
	e := &r.Entities[id]
	if r.Packages[e.Pkg].Entry != 0 && slices.Contains(r.entryOnlyNames(), e.Name) {
		r.errAt(e.File, e.Node, cEntryNameRedeclared, e.Name)
		e.Flags |= EfPoison
		return
	}
	scope := r.Packages[e.Pkg].Scope
	if _, dup := r.define(scope, e.Name, Binding{Ent: id}); dup {
		r.errAt(e.File, e.Node, cReexportConflict, e.Name)
		e.Flags |= EfPoison
	}
}

// A re-export may not be wider than the declaration it names (PKG-9).
func (r *Result) checkReexportWidening(id EntityID) {
	e := &r.Entities[id]
	target := e.Target
	if target == 0 {
		return
	}
	source := r.Entities[target].Vis
	if r.Entities[target].Kind == EntPackage {
		return
	}
	if !r.covers(source, e.Vis) {
		r.errAt(e.File, e.Node, cReexportWidens, e.Name, r.visText(source), r.visText(e.Vis))
	}
}
