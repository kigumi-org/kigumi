package sem

import "kigumi/internal/syntax"

// typeMember resolves `Type.name`: an associated function or a variant.
func (c *checker) typeMember(n syntax.NodeID, base headRef, name string) headRef {
	e := &c.r.Entities[base.ent]
	if e.Kind != EntType {
		c.errAt(n, cMemberNotFound, e.Name, name)
		return headRef{kind: headNone}
	}
	info := c.r.typeDecl(base.ent)
	if set, ok := info.Members[name]; ok {
		members := c.r.Overloads[set].Members
		for _, m := range members {
			c.r.useEntity(c.f, n, m)
		}
		if len(members) == 1 && c.r.Fn(members[0]).Recv != RecvNone {
			c.errAt(n, cAssocIsMethod, name, name)
			return headRef{kind: headNone}
		}
		if len(members) == 1 {
			c.info.Uses[n] = members[0]
			return headRef{kind: headFn, ent: members[0], set: set, args: base.args}
		}
		return headRef{kind: headSet, set: set, args: base.args}
	}
	for _, v := range info.Variants {
		if c.r.Entities[v].Name == name {
			c.r.useEntity(c.f, n, v)
			c.info.Uses[n] = v
			return headRef{kind: headVariant, ent: v, args: base.args}
		}
	}
	if c.r.findField(base.ent, name) != 0 {
		c.errAt(n, cFieldOnType, name)
		return headRef{kind: headNone}
	}
	c.errSuggest(n, cMemberNotFound, name, c.r.memberNames(base.ent), e.Name, name)
	return headRef{kind: headNone}
}
