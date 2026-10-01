package sem

import "kigumi/internal/syntax"

// resolveCtorPath resolves the path of a constructor pattern to a variant,
// or to a type for an existential type test.
func (c *checker) resolveCtorPath(p, path syntax.NodeID, scrutinee TypeID) (EntityID, bool) {
	segs := c.t.PathToks(path)
	name := c.t.TokText(segs[len(segs)-1])
	if len(segs) == 1 {
		if b, _, ok := c.r.lookup(c.scope, name); ok {
			ent := c.r.follow(b.Ent)
			if b.Ent != 0 && (c.r.Entities[ent].Kind == EntVariant || c.r.Entities[ent].Kind == EntType || c.r.Entities[ent].Kind == EntAlias) {
				return c.checkBareVariant(p, ent, scrutinee), true
			}
		}
		if vs := c.r.variantNames[c.pkg][name]; len(vs) == 1 {
			return vs[0], true
		} else if len(vs) > 1 {
			c.errAt(p, cVariantAmbiguous, name, c.r.entityName(c.r.Entities[vs[0]].Parent), c.r.entityName(c.r.Entities[vs[1]].Parent))
			return 0, false
		}
		if c.foreignVariantHint(p, name, scrutinee) {
			return 0, false
		}
		c.errAt(p, cUnresolvedConstructor, name)
		return 0, false
	}
	head := c.r.resolveValueName(c.f, c.scope, c.prefixPath(path))
	if head == 0 {
		return 0, false
	}
	switch c.r.Entities[head].Kind {
	case EntType:
		for _, v := range c.r.typeDecl(head).Variants {
			if c.r.Entities[v].Name == name {
				c.r.useEntity(c.f, p, v)
				return v, true
			}
		}
		c.errAt(p, cPatternWrongType, name, c.r.Entities[head].Name)
	case EntImport:
		if c.r.Entities[head].Target != 0 && c.r.Entities[c.r.Entities[head].Target].Kind == EntPackage {
			pkg := c.r.Entities[c.r.Entities[head].Target].Pkg
			if b, ok := c.r.Scopes[c.r.Packages[pkg].Scope].Names[name]; ok && b.Ent != 0 {
				ent := c.r.follow(b.Ent)
				if c.r.useEntity(c.f, p, ent) {
					return ent, true
				}
				return 0, false
			}
			c.errAt(p, cUnresolvedMember, c.r.Packages[pkg].Path, name)
		}
	default:
		c.errAt(p, cUnresolvedConstructor, name)
	}
	return 0, false
}

// checkBareVariant enforces that a bare variant name resolves only for the
// current package's ADTs and for Option/Result.
func (c *checker) checkBareVariant(p syntax.NodeID, ent EntityID, scrutinee TypeID) EntityID {
	e := &c.r.Entities[ent]
	if e.Kind != EntVariant || e.File == 0 || e.Pkg == c.pkg {
		return ent
	}
	owner := c.r.entityName(e.Parent)
	c.errFix(p, c.replaceNode(p, "Write `"+owner+"."+e.Name+"`", owner+"."+e.Name), cVariantBareForeign, e.Name, owner, owner, e.Name)
	return 0
}

func (c *checker) foreignVariantHint(p syntax.NodeID, name string, scrutinee TypeID) bool {
	n := c.r.Types.Node(scrutinee)
	if n.Kind != KNamed {
		return false
	}
	for _, v := range c.r.typeDecl(n.Ent).Variants {
		if c.r.Entities[v].Name == name {
			owner := c.r.entityName(n.Ent)
			c.errFix(p, c.replaceNode(p, "Write `"+owner+"."+name+"`", owner+"."+name), cVariantBareForeign, name, owner, owner, name)
			return true
		}
	}
	return false
}

func (c *checker) prefixPath(path syntax.NodeID) syntax.NodeID {
	t := c.t
	segs := t.PathToks(path)
	start := uint32(len(t.Extra))
	t.Extra = append(t.Extra, segs[:len(segs)-1]...)
	id := syntax.NodeID(len(t.Nodes))
	t.Nodes = append(t.Nodes, syntax.Node{Kind: syntax.Path, Tok: segs[0], Lhs: start, Rhs: uint32(len(t.Extra))})
	c.growTables()
	return id
}

// growTables extends side tables after a synthetic node is added.
func (c *checker) growTables() {
	for len(c.info.Types) < len(c.t.Nodes) {
		c.info.Types = append(c.info.Types, 0)
		c.info.Uses = append(c.info.Uses, 0)
		c.info.Defs = append(c.info.Defs, 0)
		c.info.Scopes = append(c.info.Scopes, 0)
	}
}

func (c *checker) ctorPattern(p syntax.NodeID, scrutinee TypeID, seen map[string]syntax.NodeID) PatInfo {
	node := c.t.Nodes[p]
	tt := c.r.Types
	subs := c.t.Children(syntax.NodeID(node.Rhs))
	ent, ok := c.resolveCtorPath(p, syntax.NodeID(node.Lhs), scrutinee)
	if !ok || ent == 0 {
		for _, s := range subs {
			c.patternInner(s, TyPoison, false, seen)
		}
		c.setType(p, TyPoison)
		return PatInfo{Kind: PatWild, Type: TyPoison}
	}
	c.info.Uses[p] = ent
	e := &c.r.Entities[ent]
	if e.Kind != EntVariant {
		return c.typeTest(p, ent, scrutinee, subs, seen)
	}
	adt := e.Parent
	sn := tt.Node(scrutinee)
	if scrutinee != TyPoison && (sn.Kind != KNamed || sn.Ent != adt) {
		c.errAt(p, cPatternWrongType, e.Name, scrutinee)
		for _, s := range subs {
			c.patternInner(s, TyPoison, false, seen)
		}
		c.setType(p, TyPoison)
		return PatInfo{Kind: PatVariant, Variant: ent, Type: scrutinee}
	}
	vi := c.r.variant(ent)
	if len(subs) != len(vi.Payload) {
		c.errAt(p, cPatternArity, e.Name, len(vi.Payload), plural(len(vi.Payload)), len(subs), plural(len(subs)))
		c.setType(p, TyPoison)
	}
	subst := map[EntityID]TypeID{}
	for i, prm := range c.r.typeDecl(adt).Params {
		if i < len(sn.Args) {
			subst[prm] = sn.Args[i]
		}
	}
	moves := false
	for i, s := range subs {
		pt := TyPoison
		if i < len(vi.Payload) {
			pt = tt.Subst(vi.Payload[i], subst)
		}
		if c.patternInner(s, pt, false, seen).Moves {
			moves = true
		}
	}
	return PatInfo{Kind: PatVariant, Variant: ent, Type: scrutinee, Moves: moves}
}

// typeTest handles `e is T` / `e is T(v)` on an interface existential.
func (c *checker) typeTest(p syntax.NodeID, ent EntityID, scrutinee TypeID, subs []syntax.NodeID, seen map[string]syntax.NodeID) PatInfo {
	tt := c.r.Types
	if scrutinee != TyPoison && tt.Kind(scrutinee) != KIface {
		c.errAt(p, cTypeTestScrutinee, scrutinee)
		c.setType(p, TyPoison)
	}
	if len(subs) > 1 {
		c.errAt(p, cTypeTestArgs)
	}
	target := TyPoison
	if c.r.Entities[ent].Kind == EntType {
		target = c.r.ownerInstance(ent)
	} else {
		target = c.r.aliasType(ent)
	}
	if scrutinee != TyPoison && tt.Kind(scrutinee) == KIface && target != TyPoison && !c.r.conforms(target, scrutinee, c.pkg).ok {
		c.errAt(p, cTypeTestNever, target, scrutinee)
	}
	for _, s := range subs {
		c.patternInner(s, target, false, seen)
	}
	return PatInfo{Kind: PatTypeTest, Type: target}
}
