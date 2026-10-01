package sem

import (
	"strings"

	"kigumi/internal/syntax"
)

// resolveVis decodes a Visibility node of a declaration in package pkg
// A missing node is package-private.
func (r *Result) resolveVis(f FileID, node syntax.NodeID, pkg PackageID) Visibility {
	if node == 0 {
		return Visibility{Level: VisPrivate, Scope: pkg}
	}
	t := r.tree(f)
	kind, _ := t.VisKind(node)
	switch kind {
	case "pub":
		return Visibility{Level: VisPub}
	case "module":
		return Visibility{Level: VisModule, Scope: pkg}
	case "super":
		parent := r.Packages[pkg].Parent
		if parent == 0 {
			r.errAt(f, node, cVisSuperAtRoot)
			return Visibility{Level: VisPrivate, Scope: pkg}
		}
		return Visibility{Level: VisSuper, Scope: parent}
	case "in":
		r.errAt(f, node, cVisInRemoved)
		return Visibility{Level: VisPrivate, Scope: pkg}
	}
	return Visibility{Level: VisPrivate, Scope: pkg}
}

// visibleFrom reports whether a declaration with visibility v can be used
// from package from.
func (r *Result) visibleFrom(v Visibility, from PackageID) bool {
	switch v.Level {
	case VisPub:
		return true
	case VisModule:
		return r.sameModule(v.Scope, from)
	case VisSuper, VisIn:
		return from == v.Scope || r.isAncestor(v.Scope, from)
	}
	return from == v.Scope
}

// covers reports whether the range of outer contains the range of inner.
func (r *Result) covers(outer, inner Visibility) bool {
	switch outer.Level {
	case VisPub:
		return true
	case VisModule:
		return inner.Level != VisPub && r.sameModule(outer.Scope, inner.Scope)
	case VisSuper, VisIn:
		if inner.Level == VisPub || inner.Level == VisModule {
			return false
		}
		return inner.Scope == outer.Scope || r.isAncestor(outer.Scope, inner.Scope)
	}
	return inner.Level == VisPrivate && inner.Scope == outer.Scope
}

func (r *Result) visText(v Visibility) string {
	switch v.Level {
	case VisIn:
		return "pub(in " + r.Packages[v.Scope].Path + ")"
	default:
		return v.Level.String()
	}
}

// useEntity checks that ent is visible from the package of file f and
// reports not-visible at node otherwise.
func (r *Result) useEntity(f FileID, node syntax.NodeID, ent EntityID) bool {
	if ent == 0 {
		return false
	}
	e := &r.Entities[ent]
	e.Flags |= EfUsed
	from := r.packageOf(f)
	if e.Pkg == 0 || r.visibleFrom(e.Vis, from) {
		return true
	}
	r.errAt(f, node, cNotVisible, e.Name, r.visText(e.Vis), r.Packages[e.Pkg].Path)
	return false
}

// joinPath joins path segments with "/".
func joinPath(segs []string) string { return strings.Join(segs, "/") }
