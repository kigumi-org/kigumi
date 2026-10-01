package sem

import "kigumi/internal/syntax"

// declareAttrs validates `@impl(...)` targets and `layout(...)` clauses and
// rejects every other attribute.
func (r *Result) declareAttrs() {
	for id := 1; id < len(r.Entities); id++ {
		e := &r.Entities[id]
		if e.File == 0 || e.Flags&EfPoison != 0 {
			continue
		}
		t := r.tree(e.File)
		switch e.Kind {
		case EntType, EntAlias:
			s := typeDecl(t, e.Node)
			r.implAttrs(EntityID(id), s.Attrs)
			if e.Kind == EntType {
				info := r.typeDecl(EntityID(id))
				info.Metadata = r.metaAttrs(e.File, s.Attrs, true)
				for _, fld := range info.Fields {
					r.checkSchemas(e.File, r.field(fld).Metadata)
				}
			}
		case EntFn:
			if r.Entities[e.Parent].Kind == EntInterface {
				continue
			}
			info := r.Fn(EntityID(id))
			info.Metadata = r.metaAttrs(e.File, fnDecl(t, e.Node).Attrs, false)
			for _, p := range info.Params {
				r.local(p).Metadata = r.metaAttrs(e.File, param(t, r.Entities[p].Node).Attrs, false)
			}
		case EntInterface:
			r.rejectAttrs(e.File, ifaceDecl(t, e.Node).Attrs)
		case EntConst:
			r.constInfo(EntityID(id)).Metadata = r.metaAttrs(e.File, constDecl(t, e.Node).Attrs, false)
		}
	}
}

func (r *Result) implAttrs(id EntityID, list syntax.NodeID) {
	if list == 0 {
		return
	}
	e := &r.Entities[id]
	f := e.File
	t := r.tree(f)
	scope := r.declScope(id)
	for _, a := range t.Children(list) {
		name := pathText(t, syntax.NodeID(t.Nodes[a].Lhs), ".")
		if name != "impl" {
			// Dotted metadata is collected by metaAttrs.
			if len(t.PathToks(syntax.NodeID(t.Nodes[a].Lhs))) == 1 {
				r.errAt(f, a, cAttrUnknown, name)
			}
			continue
		}
		if e.Kind == EntAlias {
			r.errAt(f, a, cImplOnAlias)
			continue
		}
		for _, arg := range t.Children(syntax.NodeID(t.Nodes[a].Rhs)) {
			if ty, ok := r.implTarget(f, scope, arg); ok {
				r.typeDecl(id).Impls = append(r.typeDecl(id).Impls, ImplAssert{Iface: ty, Node: arg})
			}
		}
	}
}

// implTarget reads an `@impl` argument naming an interface instance.
func (r *Result) implTarget(f FileID, scope ScopeID, n syntax.NodeID) (TypeID, bool) {
	ent, args, ok := r.exprAsTypeRef(f, scope, n)
	if !ok {
		return 0, false
	}
	switch r.Entities[ent].Kind {
	case EntInterface:
		if !r.checkArity(f, n, ent, len(r.iface(ent).Params), len(args)) {
			return 0, false
		}
		return r.Types.Iface(ent, args), true
	case EntConstraint:
		r.errAt(f, n, cImplCompilerKnown, r.Entities[ent].Name)
	default:
		r.errAt(f, n, cImplNotInterface, r.Entities[ent].Name)
	}
	return 0, false
}
