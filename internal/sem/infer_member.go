package sem

import "kigumi/internal/syntax"

func (c *checker) synthMember(n syntax.NodeID) TypeID {
	node := c.t.Nodes[n]
	name := c.t.TokText(node.Tok)
	base := syntax.NodeID(node.Lhs)
	head := c.resolveHead(base)
	switch head.kind {
	case headPkg:
		member := c.packageMember(n, head.pkg, name)
		return c.headValue(n, member)
	case headType:
		member := c.typeMember(n, head, name)
		return c.headValue(n, member)
	case headNone:
		return TyPoison
	}
	recv := c.vars.resolve(c.synth(base))
	return c.memberOn(n, base, recv, name)
}

func (c *checker) headValue(n syntax.NodeID, h headRef) TypeID {
	switch h.kind {
	case headFn:
		return c.fnItemValue(n, h.set)
	case headSet:
		c.errAt(n, cOverloadValueAmbig, c.r.Overloads[h.set].Name)
		return TyPoison
	case headVariant:
		return c.variantValue(n, h.ent, h.args)
	case headType:
		c.errAt(n, cTypeAsValue, c.r.Entities[h.ent].Name)
	case headPkg:
		c.errAt(n, cPackageAsValue, c.r.Entities[h.ent].Name, c.r.Entities[h.ent].Name)
	case headConstraint:
		c.errAt(n, cConstraintAsType, c.r.Entities[h.ent].Name)
	case headValue:
		if h.ent != 0 {
			e := &c.r.Entities[h.ent]
			c.info.Uses[n] = h.ent
			if e.Kind == EntConst {
				return e.Type
			}
			return e.Type
		}
	}
	return TyPoison
}

func (c *checker) memberOn(n, base syntax.NodeID, recv TypeID, name string) TypeID {
	tt := c.r.Types
	if recv == TyPoison {
		return TyPoison
	}
	node := tt.Node(recv)
	if node.Kind == KRef {
		recv = node.Elem
		node = tt.Node(recv)
	}
	if node.Kind == KNamed {
		if fld := c.r.findField(node.Ent, name); fld != 0 {
			if !c.r.useEntity(c.f, n, fld) {
				return TyPoison
			}
			c.info.Uses[n] = fld
			c.placeOf(n)
			return c.fieldType(recv, fld)
		}
	}
	if set := c.r.memberSet(recv, name); set != 0 {
		members := c.r.Overloads[set].Members
		for _, m := range members {
			c.r.useEntity(c.f, n, m)
		}
		if len(members) != 1 {
			c.errAt(n, cOverloadValueAmbig, name)
			return TyPoison
		}
		m := members[0]
		if c.r.Fn(m).Recv == RecvNone {
			c.errAt(n, cMethodAsAssoc, name, c.r.Entities[node.Ent].Name, name)
			return TyPoison
		}
		return c.methodValue(n, base, recv, m)
	}
	if req, sig, ambiguous := c.ifaceMember(n, recv, name); req != 0 {
		c.info.Uses[n] = req
		c.info.Calls[n] = CallInfo{Kind: CallMethodValue, Callee: req, Recv: c.r.Fn(req).Recv}
		return sig
	} else if ambiguous {
		return TyPoison
	}
	if node.Kind == KParam {
		c.errAt(n, cParamNoMember, recv, name)
		return TyPoison
	}
	c.errSuggest(n, cMemberNotFound, name, c.r.memberNames(c.r.namedEnt(recv)), recv, name)
	return TyPoison
}

func (c *checker) fieldType(recv TypeID, fld EntityID) TypeID {
	node := c.r.Types.Node(recv)
	owner := c.r.Entities[fld].Parent
	params := c.r.typeDecl(owner).Params
	if len(params) == 0 {
		return c.r.Entities[fld].Type
	}
	subst := map[EntityID]TypeID{}
	for i, p := range params {
		if i < len(node.Args) {
			subst[p] = node.Args[i]
		}
	}
	return c.r.Types.Subst(c.r.Entities[fld].Type, subst)
}
