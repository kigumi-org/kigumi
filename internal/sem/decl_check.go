package sem

import "kigumi/internal/syntax"

// checkDeclarations runs pass 4's cross-declaration rules.
func (r *Result) checkDeclarations() {
	for id := 1; id < len(r.Entities); id++ {
		e := &r.Entities[id]
		if e.File == 0 || e.Flags&EfPoison != 0 {
			continue
		}
		switch e.Kind {
		case EntFn:
			if r.Entities[e.Parent].Kind == EntInterface {
				continue
			}
			r.checkSignatureVisibility(EntityID(id), e.Vis, r.Fn(EntityID(id)).Sig)
			if e.Name == "main" && r.Fn(EntityID(id)).Owner == 0 {
				r.checkMain(EntityID(id))
			}
		case EntField:
			r.checkSignatureVisibility(EntityID(id), r.Entities[e.Parent].Vis, e.Type)
		case EntVariant:
			for _, p := range r.variant(EntityID(id)).Payload {
				r.checkSignatureVisibility(EntityID(id), e.Vis, p)
			}
		case EntAlias:
			r.checkSignatureVisibility(EntityID(id), e.Vis, e.Type)
		case EntConst:
			r.checkSignatureVisibility(EntityID(id), e.Vis, e.Type)
		}
	}
}

// checkSignatureVisibility reports every nominal type in t whose visibility
// does not cover the declaration's (VIS-6). Fields use their own visibility
// narrowed by the container's.
func (r *Result) checkSignatureVisibility(id EntityID, vis Visibility, t TypeID) {
	if t == 0 || t == TyPoison {
		return
	}
	e := &r.Entities[id]
	if e.Kind == EntField && !r.covers(e.Vis, vis) {
		vis = e.Vis
	}
	if vis.Level == VisPrivate {
		return
	}
	for _, ent := range r.nominalEntities(t) {
		te := &r.Entities[ent]
		if te.File == 0 || r.covers(te.Vis, vis) {
			continue
		}
		r.errAt(e.File, e.Node, cSignaturePrivateType, te.Name, r.visText(te.Vis), r.visText(vis), e.Name)
	}
}

func (r *Result) nominalEntities(t TypeID) []EntityID {
	var out []EntityID
	var walk func(TypeID)
	walk = func(t TypeID) {
		if t == 0 {
			return
		}
		n := r.Types.Node(t)
		if (n.Kind == KNamed || n.Kind == KIface) && n.Ent != 0 {
			out = append(out, n.Ent)
		}
		walk(n.Elem)
		for _, a := range n.Args {
			walk(a)
		}
	}
	walk(t)
	return out
}

// checkMain enforces `fn main() -> Unit!` or `fn main(host: Host) -> Unit!`
// in the main module (ENT-5). Not gated on an entry file: a module whose main
// is written out explicitly has no script-shaped file at all.
func (r *Result) checkMain(id EntityID) {
	e := &r.Entities[id]
	if p := &r.Packages[e.Pkg]; p.Std || p.Module != "" {
		return
	}
	info := r.Fn(id)
	sig := r.Types.Node(info.Sig)
	ok := sig.Elem == r.Types.Result(TyUnit, r.Types.errorType()) && info.Recv == RecvNone && len(info.TypeParams) == 0
	switch len(sig.Args) {
	case 0:
	case 1:
		host := r.langItem(e.File, e.Node, "Host")
		ok = ok && host != 0 && sig.Args[0] == r.Types.Named(host, nil)
	default:
		ok = false
	}
	if !ok {
		r.errAt(e.File, e.Node, cMainSignature)
	}
}

// resolveValueName resolves a Path used as a value head (metadata heads,
// `pkg.name`), returning the entity or 0 after reporting.
func (r *Result) resolveValueName(f FileID, scope ScopeID, path syntax.NodeID) EntityID {
	t := r.tree(f)
	segs := t.PathToks(path)
	first := t.TokText(segs[0])
	b, _, ok := r.lookup(scope, first)
	if !ok {
		r.errAt(f, path, cUnresolvedName, first)
		return 0
	}
	cur := b
	for i := 1; i < len(segs); i++ {
		if cur.Ent == 0 {
			r.errAt(f, path, cUnresolvedName, t.TokText(segs[i]))
			return 0
		}
		e := &r.Entities[cur.Ent]
		if e.Kind != EntImport || e.Target == 0 || r.Entities[e.Target].Kind != EntPackage {
			r.errAt(f, path, cUnresolvedMember, e.Name, t.TokText(segs[i]))
			return 0
		}
		e.Flags |= EfUsed
		pkg := r.Entities[e.Target].Pkg
		next, ok := r.Scopes[r.Packages[pkg].Scope].Names[t.TokText(segs[i])]
		if !ok {
			r.errAt(f, path, cUnresolvedMember, r.Packages[pkg].Path, t.TokText(segs[i]))
			return 0
		}
		cur = next
	}
	if cur.Ent == 0 {
		members := r.Overloads[cur.Set].Members
		for _, m := range members {
			r.useEntity(f, path, m)
		}
		return members[0]
	}
	ent := r.follow(cur.Ent)
	if !r.useEntity(f, path, ent) {
		return 0
	}
	return ent
}
