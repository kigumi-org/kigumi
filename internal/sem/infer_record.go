package sem

import "kigumi/internal/syntax"

// synthRecord checks a record literal `T { field: v, ..spread }`.
func (c *checker) synthRecord(n syntax.NodeID, want TypeID) TypeID {
	node := c.t.Nodes[n]
	headNode := syntax.NodeID(node.Lhs)
	entries := c.t.Children(syntax.NodeID(node.Rhs))
	head := c.resolveHead(headNode)
	switch head.kind {
	case headType:
	case headVariant:
		c.errAt(headNode, cRecordLitVariant, c.r.Entities[head.ent].Name, c.r.Entities[head.ent].Name)
		c.synthEntries(entries)
		return TyPoison
	case headNone:
		c.synthEntries(entries)
		return TyPoison
	default:
		c.errAt(headNode, cRecordLitNotRecord, c.placeName(headNode), "not a type")
		c.synthEntries(entries)
		return TyPoison
	}
	ent := head.ent
	if c.r.Entities[ent].Kind == EntAlias {
		if t := c.r.aliasType(ent); c.r.Types.Kind(t) == KNamed {
			ent = c.r.Types.Node(t).Ent
			head.args = c.r.Types.Node(t).Args
		}
	}
	e := &c.r.Entities[ent]
	if e.Kind != EntType || c.r.typeDecl(ent).Form == FormAdt || c.r.typeDecl(ent).Form == FormOpaque {
		c.errAt(headNode, cRecordLitNotRecord, e.Name, "a "+e.Kind.String())
		c.synthEntries(entries)
		return TyPoison
	}
	c.info.Uses[headNode] = ent
	info := c.r.typeDecl(ent)
	typeArgs := c.adtArgs(n, ent, head.args)
	result := c.r.Types.Named(ent, typeArgs)
	if want != 0 {
		c.unifyWant(n, result, want)
	}
	subst := map[EntityID]TypeID{}
	for i, p := range info.Params {
		subst[p] = typeArgs[i]
	}
	given := map[EntityID]bool{}
	spreadOK := true
	for _, entry := range entries {
		en := c.t.Nodes[entry]
		if en.Kind == syntax.Spread {
			spreadOK = c.spreadEntry(ent, subst, syntax.NodeID(en.Lhs), given) && spreadOK
			continue
		}
		name := c.t.TokText(en.Tok)
		fld := c.r.findField(ent, name)
		if fld == 0 {
			c.errSuggest(entry, cUnknownField, name, c.r.memberNames(ent), e.Name, name)
			c.synth(syntax.NodeID(en.Lhs))
			continue
		}
		c.r.useEntity(c.f, entry, fld)
		c.info.Uses[entry] = fld
		if given[fld] && c.t.Kind(prevEntry(c.t, entries, entry)) != syntax.Spread {
			c.errAt(entry, cDuplicateFieldInit, name)
		}
		given[fld] = true
		c.checkArg(syntax.NodeID(en.Lhs), c.r.Types.Subst(c.r.Entities[fld].Type, subst), 0)
	}
	var missing []string
	for _, fld := range info.Fields {
		if !given[fld] {
			missing = append(missing, "`"+c.r.Entities[fld].Name+"`")
		}
	}
	if len(missing) > 0 && spreadOK {
		c.errAt(n, cMissingField, plural(len(missing)), joinComma(missing), e.Name)
	}
	c.info.Calls[n] = CallInfo{Kind: CallRecord, Callee: ent, Inst: typeArgs}
	c.touched = append(c.touched, n)
	return result
}

func prevEntry(t *syntax.Tree, entries []syntax.NodeID, entry syntax.NodeID) syntax.NodeID {
	for i, e := range entries {
		if e == entry && i > 0 {
			return entries[i-1]
		}
	}
	return 0
}

func (c *checker) synthEntries(entries []syntax.NodeID) {
	for _, entry := range entries {
		c.synth(syntax.NodeID(c.t.Nodes[entry].Lhs))
	}
}

// spreadEntry projects the same-named, same-typed, Copy, visible fields of
// the spread value onto the target.
func (c *checker) spreadEntry(target EntityID, subst map[EntityID]TypeID, src syntax.NodeID, given map[EntityID]bool) bool {
	st := c.vars.resolve(c.synth(src))
	tt := c.r.Types
	if st == TyPoison {
		return false
	}
	if tt.Kind(st) == KUntyped {
		st = defaultOf(st)
	}
	// Spreading a borrow (`self`, `&S`, `&mut S`) is a logical copy when the
	// referent is Copy: reading its fields can't alias the borrowed value.
	if tt.Kind(st) == KRef {
		if elem := tt.Node(st).Elem; tt.Kind(elem) == KNamed && c.r.isCopy(elem) {
			st = elem
		}
	}
	if tt.Kind(st) != KNamed || c.r.typeDecl(tt.Node(st).Ent).Form == FormAdt || c.r.typeDecl(tt.Node(st).Ent).Form == FormOpaque {
		c.errAt(src, cSpreadNotRecord, st)
		return false
	}
	srcEnt := tt.Node(st).Ent
	for _, fld := range c.r.typeDecl(target).Fields {
		name := c.r.Entities[fld].Name
		sf := c.r.findField(srcEnt, name)
		if sf == 0 {
			continue
		}
		want := tt.Subst(c.r.Entities[fld].Type, subst)
		got := c.fieldType(st, sf)
		switch {
		case !c.r.visibleFrom(c.r.Entities[sf].Vis, c.pkg):
			c.errAt(src, cNotVisible, name, c.r.visText(c.r.Entities[sf].Vis), c.r.Packages[c.r.Entities[sf].Pkg].Path)
		case got != want:
			c.errAt(src, cSpreadFieldType, name, got, want)
		}
		given[fld] = true
	}
	return true
}

func joinComma(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
