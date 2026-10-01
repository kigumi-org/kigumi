package sem

import "kigumi/internal/syntax"

// recordPattern handles `T { a, b: p }` over a record or a variant with
// named payload.
func (c *checker) recordPattern(p syntax.NodeID, scrutinee TypeID, seen map[string]syntax.NodeID) PatInfo {
	node := c.t.Nodes[p]
	tt := c.r.Types
	fields := c.t.Children(syntax.NodeID(node.Rhs))
	ent, ok := c.resolveCtorPath(p, syntax.NodeID(node.Lhs), scrutinee)
	if !ok || ent == 0 {
		c.skipFields(fields, seen)
		c.setType(p, TyPoison)
		return PatInfo{Kind: PatWild, Type: TyPoison}
	}
	c.info.Uses[p] = ent
	e := &c.r.Entities[ent]
	sn := tt.Node(scrutinee)
	subst := map[EntityID]TypeID{}
	var info PatInfo
	var lookup func(name string) (TypeID, EntityID, bool)
	switch {
	case e.Kind == EntType && c.r.typeDecl(ent).Form != FormAdt:
		if scrutinee != TyPoison && (sn.Kind != KNamed || sn.Ent != ent) {
			c.errAt(p, cPatternKind, scrutinee)
			c.skipFields(fields, seen)
			c.setType(p, TyPoison)
			return PatInfo{Kind: PatWild, Type: scrutinee}
		}
		for i, prm := range c.r.typeDecl(ent).Params {
			if i < len(sn.Args) {
				subst[prm] = sn.Args[i]
			}
		}
		info = PatInfo{Kind: PatRecordTy, Type: scrutinee}
		lookup = func(name string) (TypeID, EntityID, bool) {
			fld := c.r.findField(ent, name)
			if fld == 0 {
				return 0, 0, false
			}
			return tt.Subst(c.r.Entities[fld].Type, subst), fld, true
		}
	case e.Kind == EntVariant:
		if e.Flags&EfNamedPayload == 0 {
			c.errAt(p, cPatternRecordPositional, e.Name, e.Name)
			c.skipFields(fields, seen)
			c.setType(p, TyPoison)
			return PatInfo{Kind: PatVariant, Variant: ent, Type: scrutinee}
		}
		if scrutinee != TyPoison && (sn.Kind != KNamed || sn.Ent != e.Parent) {
			c.errAt(p, cPatternWrongType, e.Name, scrutinee)
			c.skipFields(fields, seen)
			c.setType(p, TyPoison)
			return PatInfo{Kind: PatVariant, Variant: ent, Type: scrutinee}
		}
		for i, prm := range c.r.typeDecl(e.Parent).Params {
			if i < len(sn.Args) {
				subst[prm] = sn.Args[i]
			}
		}
		vi := c.r.variant(ent)
		info = PatInfo{Kind: PatVariant, Variant: ent, Type: scrutinee}
		lookup = func(name string) (TypeID, EntityID, bool) {
			for i, n := range vi.Names {
				if n == name {
					return tt.Subst(vi.Payload[i], subst), 0, true
				}
			}
			return 0, 0, false
		}
	default:
		c.errAt(p, cPatternKind, scrutinee)
		c.skipFields(fields, seen)
		c.setType(p, TyPoison)
		return PatInfo{Kind: PatWild, Type: scrutinee}
	}
	used := map[string]bool{}
	for _, f := range fields {
		fn := c.t.Nodes[f]
		name := c.t.TokText(fn.Tok)
		ft, fld, ok := lookup(name)
		if !ok {
			c.errAt(f, cPatternUnknownField, e.Name, name)
			c.info.Types[p] = TyPoison
			if fn.Lhs != 0 {
				c.patternInner(syntax.NodeID(fn.Lhs), TyPoison, false, seen)
			}
			continue
		}
		if used[name] {
			c.errAt(f, cPatternDuplicateField, name)
		}
		used[name] = true
		if fld != 0 {
			c.r.useEntity(c.f, f, fld)
			c.info.Uses[f] = fld
			info.Fields = append(info.Fields, fld)
		}
		c.setType(f, ft)
		if fn.Lhs != 0 {
			if c.patternInner(syntax.NodeID(fn.Lhs), ft, false, seen).Moves {
				info.Moves = true
			}
			continue
		}
		if prev, dup := seen[name]; dup && prev != f {
			c.errAt(f, cPatternDuplicateBinding, name)
		}
		seen[name] = f
		c.declareLocal(f, name, c.bindingType(ft), false)
		if !c.patBorrow && !c.r.isCopy(ft) {
			info.Moves = true
		}
	}
	return info
}

func (c *checker) skipFields(fields []syntax.NodeID, seen map[string]syntax.NodeID) {
	for _, f := range fields {
		if sub := syntax.NodeID(c.t.Nodes[f].Lhs); sub != 0 {
			c.patternInner(sub, TyPoison, false, seen)
		}
	}
}

// synthIs types `e is Pattern`: a Bool whose pattern bindings are
// visible to the rest of the enclosing condition and then-block.
func (c *checker) synthIs(n syntax.NodeID) TypeID {
	node := c.t.Nodes[n]
	scrutinee := c.vars.resolve(c.synth(syntax.NodeID(node.Lhs)))
	if c.r.Types.Kind(scrutinee) == KUntyped {
		scrutinee = c.adopt(c.literalNode(syntax.NodeID(node.Lhs)), defaultOf(scrutinee))
	}
	if c.inCond == 0 {
		saved := c.scope
		c.pushScope(ScopeArm, n)
		c.checkPatternOn(syntax.NodeID(node.Rhs), syntax.NodeID(node.Lhs), scrutinee, false, c.scope)
		if len(c.r.Scopes[c.scope].Names) > 0 {
			c.errAt(n, cIsBindingOutsideCond)
		}
		c.popScope(saved)
	} else {
		c.checkPatternOn(syntax.NodeID(node.Rhs), syntax.NodeID(node.Lhs), scrutinee, false, c.scope)
	}
	c.info.Narrow[n] = c.snapshotFacts()
	c.info.Matches[n] = MatchInfo{}
	return TyBool
}
