package sem

import "kigumi/internal/syntax"

func (r *Result) resolveTypeName(f FileID, scope ScopeID, path syntax.NodeID) EntityID {
	t := r.tree(f)
	segs := t.PathToks(path)
	first := t.TokText(segs[0])
	b, _, ok := r.lookup(scope, first)
	if !ok {
		if first == "Self" {
			r.errAt(f, path, cSelfOutsideInterface)
		} else {
			r.errAt(f, path, cUnresolvedName, first)
		}
		return 0
	}
	if b.Ent == 0 {
		r.errAt(f, path, cNotAType, first, "a function")
		return 0
	}
	if len(segs) == 1 {
		return r.follow(b.Ent)
	}
	e := &r.Entities[b.Ent]
	if e.Kind != EntImport || e.Target == 0 || r.Entities[e.Target].Kind != EntPackage {
		r.errAt(f, path, cNotAType, first, "not a package")
		return 0
	}
	e.Flags |= EfUsed
	pkg := r.Entities[e.Target].Pkg
	second := t.TokText(segs[1])
	member, ok := r.Scopes[r.Packages[pkg].Scope].Names[second]
	if !ok {
		r.errAt(f, path, cUnresolvedMember, r.Packages[pkg].Path, second)
		return 0
	}
	if member.Ent == 0 {
		r.errAt(f, path, cNotAType, second, "a function")
		return 0
	}
	if len(segs) > 2 {
		r.errAt(f, path, cNotAType, pathText(t, path, "."), "not a type path")
		return 0
	}
	ent := r.follow(member.Ent)
	if !r.useEntity(f, path, ent) {
		return 0
	}
	return ent
}

func (r *Result) follow(id EntityID) EntityID {
	for id != 0 {
		e := &r.Entities[id]
		if e.Kind != EntImport || e.Target == 0 || r.Entities[e.Target].Kind == EntPackage {
			return id
		}
		e.Flags |= EfUsed
		id = e.Target
	}
	return id
}
