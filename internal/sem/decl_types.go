package sem

import "kigumi/internal/syntax"

// declareTypes resolves type declarations, interfaces, and aliases (pass 3).
func (r *Result) declareTypes() {
	var types, ifaces, aliases []EntityID
	for id := 1; id < len(r.Entities); id++ {
		e := &r.Entities[id]
		if e.File == 0 || e.Flags&EfPoison != 0 {
			continue
		}
		switch e.Kind {
		case EntType:
			types = append(types, EntityID(id))
		case EntInterface:
			ifaces = append(ifaces, EntityID(id))
		case EntAlias:
			aliases = append(aliases, EntityID(id))
		}
	}
	for _, id := range types {
		e := &r.Entities[id]
		s := typeDecl(r.tree(e.File), e.Node)
		r.typeDecl(id).Params = r.declareGenerics(e.File, id, s.Generics, r.declScope(id))
	}
	for _, id := range ifaces {
		r.declareIfaceBinder(id)
	}
	for _, id := range aliases {
		e := &r.Entities[id]
		s := typeDecl(r.tree(e.File), e.Node)
		if s.Generics != 0 {
			r.aliasParams[id] = r.declareGenerics(e.File, id, s.Generics, r.declScope(id))
			for _, g := range r.tree(e.File).Children(s.Generics) {
				if r.tree(e.File).Nodes[g].Rhs != 0 {
					r.errAt(e.File, g, cAliasParamBound)
				}
			}
		}
		if s.Layout != 0 {
			r.errAt(e.File, s.Layout, cLayoutTarget)
		}
	}
	for _, id := range types {
		e := &r.Entities[id]
		s := typeDecl(r.tree(e.File), e.Node)
		r.resolveConstraints(e.File, r.declScope(id), s.Generics, r.typeDecl(id).Params)
	}
	for _, id := range ifaces {
		r.declareIfaceConstraints(id)
	}
	for _, id := range aliases {
		r.aliasType(id)
	}
	for _, id := range ifaces {
		r.declareRequirements(id)
	}
	for _, id := range types {
		r.declareTypeBody(id)
	}
	for _, id := range types {
		r.checkFiniteSize(id)
		r.checkLayoutFields(id)
	}
}

func (r *Result) declareTypeBody(id EntityID) {
	e := &r.Entities[id]
	t := r.tree(e.File)
	s := typeDecl(t, e.Node)
	scope := r.declScope(id)
	r.declareLayout(e.File, id, s.Layout)
	switch t.Kind(s.Body) {
	case syntax.RecordBody, syntax.ResourceBody:
		r.declareFields(e.File, id, scope, s.Body)
	case syntax.AdtBody:
		r.declareVariants(e.File, id, scope, s.Body)
	}
}

func (r *Result) declareFields(f FileID, owner EntityID, scope ScopeID, body syntax.NodeID) {
	t := r.tree(f)
	info := r.typeDecl(owner)
	seen := map[string]bool{}
	for i, fld := range t.Children(body) {
		name := t.TokText(t.Nodes[fld].Tok)
		s := field(t, fld)
		vis := r.resolveVis(f, s.Vis, r.packageOf(f))
		id := r.newEntity(Entity{Kind: EntField, Name: name, Pkg: r.packageOf(f), File: f, Node: fld, Tok: t.Nodes[fld].Tok, Parent: owner, Vis: vis})
		if s.Flags&syntax.FlagMut != 0 {
			r.Entities[id].Flags |= EfMut
		}
		r.Files[f].Defs[fld] = id
		typ := r.resolveType(f, scope, s.Type, posField)
		r.Entities[id].Type = typ
		lt := r.explicitLifetimeOf(f, scope, s.Type)
		r.checkFieldLifetimeEscape(f, s.Type, owner, typ, lt)
		r.Entities[id].Detail = r.addField(FieldInfo{Index: i, Type: typ, Metadata: r.resolveMetadata(f, fld, s.Metadata), Lifetime: lt})
		switch {
		case name == "nil":
			r.errAt(f, fld, cNilName)
		case seen[name]:
			r.errAt(f, fld, cDuplicateField, name)
		}
		seen[name] = true
		info.Fields = append(info.Fields, id)
	}
}

func (r *Result) declareVariants(f FileID, owner EntityID, scope ScopeID, body syntax.NodeID) {
	t := r.tree(f)
	info := r.typeDecl(owner)
	seen := map[string]bool{}
	pkg := r.packageOf(f)
	for i, v := range t.Children(body) {
		name := t.TokText(t.Nodes[v].Tok)
		id := r.newEntity(Entity{Kind: EntVariant, Name: name, Pkg: pkg, File: f, Node: v, Tok: t.Nodes[v].Tok, Parent: owner, Vis: r.Entities[owner].Vis})
		r.Files[f].Defs[v] = id
		vi := VariantInfo{Index: i}
		named, positional := 0, 0
		if list := syntax.NodeID(t.Nodes[v].Lhs); list != 0 {
			for _, vf := range t.Children(list) {
				vn := t.Nodes[vf]
				vi.Payload = append(vi.Payload, r.resolveType(f, scope, syntax.NodeID(vn.Rhs), posPayload))
				if vn.Lhs&syntax.FlagNamed != 0 {
					vi.Names = append(vi.Names, t.TokText(vn.Tok))
					named++
				} else {
					vi.Names = append(vi.Names, "")
					positional++
				}
			}
		}
		if named > 0 && positional > 0 {
			r.errAt(f, v, cVariantPayloadMixed, name)
		}
		if named > 0 {
			r.Entities[id].Flags |= EfNamedPayload
		}
		if seen[name] {
			r.errAt(f, v, cDuplicateVariant, name)
		}
		seen[name] = true
		r.checkVariantPreludeCollision(f, v, name)
		r.checkVariantEntryCollision(f, v, name)
		r.Entities[id].Detail = r.addVariant(vi)
		info.Variants = append(info.Variants, id)
		if r.variantNames[pkg] == nil {
			r.variantNames[pkg] = map[string][]EntityID{}
		}
		r.variantNames[pkg][name] = append(r.variantNames[pkg][name], id)
	}
}
