package sem

import "kigumi/internal/syntax"

type headKind uint8

const (
	headValue headKind = iota
	headPkg
	headType
	headFn
	headSet
	headVariant
	headConstraint
	headIntrinsic
	headParam  // a type parameter, `T`
	headStatic // a static requirement on a type parameter, `T.fromJson`
	headNone
)

type headRef struct {
	kind headKind
	ent  EntityID
	set  OverloadSetID
	pkg  PackageID
	args []TypeID
	// param and iface locate a static requirement: the type parameter it
	// is called on and the constraint that provides it.
	param EntityID
	iface TypeID
}

func (c *checker) resolveHead(n syntax.NodeID) headRef {
	node := c.t.Nodes[n]
	switch node.Kind {
	case syntax.Ident:
		return c.identHead(n)
	case syntax.MemberExpr:
		base := c.resolveHead(syntax.NodeID(node.Lhs))
		name := c.t.TokText(node.Tok)
		switch base.kind {
		case headPkg:
			return c.packageMember(n, base.pkg, name)
		case headType:
			return c.typeMember(n, base, name)
		case headParam:
			if !c.r.typeParam(base.ent).IsConst {
				return c.paramMember(n, base, name)
			}
		}
		return headRef{kind: headValue}
	case syntax.BracketExpr:
		base := c.resolveHead(syntax.NodeID(node.Lhs))
		generic := base.kind == headType && len(c.r.typeDecl(base.ent).Params) > 0 ||
			base.kind == headFn && len(c.ownGenerics(base.ent)) > 0 ||
			base.kind == headSet && c.setHasGeneric(base.set)
		if generic && len(base.args) == 0 {
			var params []EntityID
			switch base.kind {
			case headType:
				params = c.r.typeDecl(base.ent).Params
			case headFn:
				params = c.ownGenerics(base.ent)
			}
			for i, a := range c.t.Children(syntax.NodeID(node.Rhs)) {
				base.args = append(base.args, c.r.resolveExprGenericArg(c.f, c.scope, a, entAt(params, i)))
			}
			c.info.Uses[n] = base.ent
			return base
		}
		return headRef{kind: headValue}
	}
	return headRef{kind: headValue}
}

func (c *checker) setHasGeneric(set OverloadSetID) bool {
	for _, m := range c.r.Overloads[set].Members {
		if len(c.ownGenerics(m)) > 0 {
			return true
		}
	}
	return false
}

func (c *checker) identHead(n syntax.NodeID) headRef {
	name := c.t.TokText(c.t.Nodes[n].Tok)
	b, found, ok := c.r.lookup(c.scope, name)
	if !ok {
		if variants := c.r.variantNames[c.pkg][name]; len(variants) == 1 {
			return headRef{kind: headVariant, ent: variants[0]}
		}
		c.unresolved(n, name)
		return headRef{kind: headNone}
	}
	_ = found
	if b.Ent == 0 {
		set := &c.r.Overloads[b.Set]
		if len(set.Members) == 1 {
			return headRef{kind: headFn, ent: set.Members[0], set: b.Set}
		}
		return headRef{kind: headSet, set: b.Set}
	}
	ent := c.r.follow(b.Ent)
	return c.classify(ent)
}

func (c *checker) classify(ent EntityID) headRef {
	e := &c.r.Entities[ent]
	switch e.Kind {
	case EntImport:
		if e.Target != 0 && c.r.Entities[e.Target].Kind == EntPackage {
			e.Flags |= EfUsed
			return headRef{kind: headPkg, pkg: c.r.Entities[e.Target].Pkg, ent: ent}
		}
		if e.Detail != 0 {
			set := &c.r.Overloads[OverloadSetID(e.Detail)]
			if len(set.Members) == 1 {
				return headRef{kind: headFn, ent: set.Members[0], set: OverloadSetID(e.Detail)}
			}
			return headRef{kind: headSet, set: OverloadSetID(e.Detail)}
		}
		return headRef{kind: headNone}
	case EntType, EntAlias:
		return headRef{kind: headType, ent: ent}
	case EntInterface:
		return headRef{kind: headType, ent: ent}
	case EntFn:
		if set := c.r.Fn(ent).Set; set != 0 && len(c.r.Overloads[set].Members) > 1 {
			return headRef{kind: headSet, set: set}
		}
		return headRef{kind: headFn, ent: ent}
	case EntVariant:
		return headRef{kind: headVariant, ent: ent}
	case EntConstraint:
		return headRef{kind: headConstraint, ent: ent}
	case EntTypeParam:
		return headRef{kind: headParam, ent: ent}
	case EntIntrinsic:
		return headRef{kind: headIntrinsic, ent: ent}
	}
	return headRef{kind: headValue, ent: ent}
}

func (c *checker) packageMember(n syntax.NodeID, pkg PackageID, name string) headRef {
	b, ok := c.r.Scopes[c.r.Packages[pkg].Scope].Names[name]
	if !ok {
		c.errAt(n, cUnresolvedMember, c.r.Packages[pkg].Path, name)
		return headRef{kind: headNone}
	}
	if b.Ent == 0 {
		set := &c.r.Overloads[b.Set]
		for _, m := range set.Members {
			c.r.useEntity(c.f, n, m)
		}
		if len(set.Members) == 1 {
			c.info.Uses[n] = set.Members[0]
			return headRef{kind: headFn, ent: set.Members[0], set: b.Set}
		}
		return headRef{kind: headSet, set: b.Set}
	}
	ent := c.r.follow(b.Ent)
	if !c.r.useEntity(c.f, n, ent) {
		return headRef{kind: headNone}
	}
	c.info.Uses[n] = ent
	return c.classify(ent)
}
