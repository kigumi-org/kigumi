package sem

func (g *deriver) derivable(t TypeID, iface string) bool {
	r := g.r
	if isProto(iface) {
		return r.Types.Kind(t) == KParam || r.deriveOf(t, iface)
	}
	method := deriveMethods[iface]
	n := r.Types.Node(t)
	switch n.Kind {
	case KParam:
		return true
	case KPrim:
		return r.memberSet(t, method) != 0
	case KNamed:
		for _, a := range n.Args {
			if !g.derivable(a, iface) {
				return false
			}
		}
		if r.Packages[r.Entities[n.Ent].Pkg].Std {
			return r.memberSet(t, method) != 0
		}
		if r.memberSet(t, method) != 0 || g.visiting[n.Ent] {
			return true
		}
		info := r.typeDecl(n.Ent)
		if info.Form != FormRecord && info.Form != FormAdt && info.Form != FormResource {
			return false
		}
		if g.visiting == nil {
			g.visiting = map[EntityID]bool{}
		}
		g.visiting[n.Ent] = true
		defer delete(g.visiting, n.Ent)
		for _, fld := range info.Fields {
			if !g.derivable(r.field(fld).Type, iface) {
				return false
			}
		}
		for _, v := range info.Variants {
			for _, pt := range r.variant(v).Payload {
				if !g.derivable(pt, iface) {
					return false
				}
			}
		}
		return true
	}
	return false
}

// JSON null cannot tell Some(None) from None, so Option[Option[_]] is
// rejected rather than mapped to null; the scan's own
// substitution is what catches it appearing only after a generic type's
// arguments are substituted, never in the bare declaration.
func (r *Result) jsonNestedOptionField(decl EntityID, iface string) (EntityID, bool) {
	return r.jsonNestedOptionScan(decl, iface, nil, map[EntityID]bool{decl: true})
}

// Like jsonNestedOptionField, but for a use-site type argument; also
// excludes anything derive() would skip, including a hand-written codec.
func (r *Result) jsonNestedOptionArg(t TypeID, iface string) (EntityID, bool) {
	n := r.Types.Node(t)
	if n.Kind != KNamed {
		return 0, false
	}
	info := r.typeDecl(n.Ent)
	if info.Form != FormRecord && info.Form != FormAdt {
		return 0, false
	}
	if r.hasJSONCodec(info, iface) {
		return 0, false
	}
	return r.jsonNestedOptionScan(n.Ent, iface, namedSubst(info.Params, n.Args), map[EntityID]bool{n.Ent: true})
}

// hasJSONCodec reports a HAND-WRITTEN codec; a compiler-derived one (a
// "*.derived.kg" file) is the generic, instantiation-blind codec this check
// exists to distrust, so it doesn't exempt.
func (r *Result) hasJSONCodec(info *TypeDeclInfo, iface string) bool {
	set, ok := info.Members[deriveMethods[iface]]
	if !ok {
		return false
	}
	for _, m := range r.Overloads[set].Members {
		if f := r.tree(r.Entities[m].File).File; f == nil || !f.Generated {
			return true
		}
	}
	return false
}

// subst binds decl's own type parameters at a use site (nil for a plain
// declaration). visiting guards against a self-referential type looping
// forever; it is mutated and restored in place, shared across one scan.
func (r *Result) jsonNestedOptionScan(decl EntityID, iface string, subst map[EntityID]TypeID, visiting map[EntityID]bool) (EntityID, bool) {
	info := r.typeDecl(decl)
	for _, fld := range info.Fields {
		if e, bad := r.jsonNestedOptionType(r.Types.Subst(r.field(fld).Type, subst), iface, visiting); bad {
			if e == 0 {
				e = fld
			}
			return e, true
		}
	}
	for _, v := range info.Variants {
		for _, pt := range r.variant(v).Payload {
			if e, bad := r.jsonNestedOptionType(r.Types.Subst(pt, subst), iface, visiting); bad {
				if e == 0 {
					e = v
				}
				return e, true
			}
		}
	}
	return 0, false
}

// A std type, or one with its own hand-written codec, is trusted rather
// than descended into.
func (r *Result) jsonNestedOptionType(t TypeID, iface string, visiting map[EntityID]bool) (EntityID, bool) {
	if isNestedOption(r.Types, t) {
		return 0, true
	}
	n := r.Types.Node(t)
	if n.Kind != KNamed || visiting[n.Ent] {
		return 0, false
	}
	info := r.typeDecl(n.Ent)
	if info.Form != FormRecord && info.Form != FormAdt {
		return 0, false
	}
	if r.Packages[r.Entities[n.Ent].Pkg].Std || r.hasJSONCodec(info, iface) {
		return 0, false
	}
	visiting[n.Ent] = true
	defer delete(visiting, n.Ent)
	return r.jsonNestedOptionScan(n.Ent, iface, namedSubst(info.Params, n.Args), visiting)
}

func namedSubst(params []EntityID, args []TypeID) map[EntityID]TypeID {
	if len(params) == 0 {
		return nil
	}
	subst := map[EntityID]TypeID{}
	for i, p := range params {
		if i < len(args) {
			subst[p] = args[i]
		}
	}
	return subst
}

func isNestedOption(tt *TypeTable, t TypeID) bool {
	inner, ok := tt.IsOption(t)
	if !ok {
		return false
	}
	_, ok = tt.IsOption(inner)
	return ok
}
