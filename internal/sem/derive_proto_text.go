package sem

import (
	"fmt"
	"strings"
)

func (r *Result) derivable(t TypeID, proto string) bool {
	n := r.Types.Node(t)
	if n.Kind != KNamed {
		return false
	}
	if form := r.typeDecl(n.Ent).Form; form != FormRecord && form != FormAdt {
		return false
	}
	return r.deriveOf(t, proto)
}

func (g *deriver) proto(d deriveRequest, info *TypeDeclInfo) string {
	if d.iface == "Eq" {
		return g.equals(d, info)
	}
	return g.hashOf(d, info)
}

// vis mirrors the type's visibility so the generated method is usable from
// other packages.
func (g *deriver) vis(d deriveRequest) string {
	if g.r.Entities[d.decl].Vis.Level == VisPub {
		return "pub "
	}
	return ""
}

func (g *deriver) protoBinder(d deriveRequest, info *TypeDeclInfo) string {
	name := g.r.Entities[d.decl].Name
	if len(info.Params) == 0 {
		return name
	}
	var ps []string
	for _, p := range info.Params {
		if g.r.typeParam(p).IsConst {
			ps = append(ps, g.r.Entities[p].Name)
			continue
		}
		ps = append(ps, g.r.Entities[p].Name+": "+d.iface)
	}
	return name + "[" + strings.Join(ps, ", ") + "]"
}

func (g *deriver) equals(d deriveRequest, info *TypeDeclInfo) string {
	var sb strings.Builder
	name := g.r.Entities[d.decl].Name
	fmt.Fprintf(&sb, "%spure fn %s.equals(self, other: &%s) -> Bool {\n", g.vis(d), g.protoBinder(d, info), g.instance(d, info))
	if info.Form != FormAdt {
		var terms []string
		for _, fld := range info.Fields {
			fname := g.r.Entities[fld].Name
			// Field access is never itself a reference, so `.equals(&other.f)`
			// is always the right shape; going through the protocol (not `==`)
			// gives a Float field total Eq rather than raw IEEE `==`.
			terms = append(terms, fmt.Sprintf("self.%s.equals(&other.%s)", fname, fname))
		}
		if len(terms) == 0 {
			terms = []string{"true"}
		}
		fmt.Fprintf(&sb, "    %s\n}\n", strings.Join(terms, " && "))
		return sb.String()
	}
	sb.WriteString("    match self {\n")
	for _, v := range info.Variants {
		vname := g.r.Entities[v].Name
		payload := g.r.variant(v).Payload
		n := len(payload)
		if n == 0 {
			fmt.Fprintf(&sb, "        %s.%s -> other is %s.%s\n", name, vname, name, vname)
			continue
		}
		var mine, theirs, terms []string
		for i := range n {
			mine = append(mine, fmt.Sprintf("a%d", i))
			theirs = append(theirs, fmt.Sprintf("b%d", i))
			// Binding mode: a Copy payload binds by value and needs `&`;
			// a non-Copy payload already binds as a borrow, matching what
			// `.equals` wants.
			arg := fmt.Sprintf("b%d", i)
			if g.r.isCopy(payload[i]) {
				arg = "&" + arg
			}
			terms = append(terms, fmt.Sprintf("a%d.equals(%s)", i, arg))
		}
		fmt.Fprintf(&sb, "        %s.%s(%s) -> other is %s.%s(%s) && %s\n", name, vname, strings.Join(mine, ", "), name, vname, strings.Join(theirs, ", "), strings.Join(terms, " && "))
	}
	sb.WriteString("    }\n}\n")
	return sb.String()
}

// hashOf mixes the parts with FNV-1a; an ADT starts from its tag.
func (g *deriver) hashOf(d deriveRequest, info *TypeDeclInfo) string {
	var sb strings.Builder
	name := g.r.Entities[d.decl].Name
	fmt.Fprintf(&sb, "%spure fn %s.hash(self) -> u64 {\n", g.vis(d), g.protoBinder(d, info))
	mix := func(indent, expr string) {
		fmt.Fprintf(&sb, "%sh = (h ^ %s).wrappingMul(1099511628211)\n", indent, expr)
	}
	if info.Form != FormAdt {
		sb.WriteString("    let mut h: u64 = 14695981039346656037\n")
		for _, fld := range info.Fields {
			mix("    ", fmt.Sprintf("self.%s.hash()", g.r.Entities[fld].Name))
		}
		sb.WriteString("    h\n}\n")
		return sb.String()
	}
	sb.WriteString("    match self {\n")
	for tag, v := range info.Variants {
		vname := g.r.Entities[v].Name
		n := len(g.r.variant(v).Payload)
		if n == 0 {
			fmt.Fprintf(&sb, "        %s.%s -> %d\n", name, vname, tag+1)
			continue
		}
		var binds []string
		for i := range n {
			binds = append(binds, fmt.Sprintf("p%d", i))
		}
		fmt.Fprintf(&sb, "        %s.%s(%s) -> {\n            let mut h: u64 = %d\n", name, vname, strings.Join(binds, ", "), tag+1)
		for i := range n {
			mix("            ", fmt.Sprintf("p%d.hash()", i))
		}
		sb.WriteString("            h\n        }\n")
	}
	sb.WriteString("    }\n}\n")
	return sb.String()
}

// protocolMismatch explains why t's own method fails the requirement, or
// "" if there is no such method.
func (r *Result) protocolMismatch(t TypeID, proto string) string {
	n := r.Types.Node(t)
	iface := r.langItem(0, 0, proto)
	if n.Kind != KNamed || iface == 0 {
		return ""
	}
	set, ok := r.typeDecl(n.Ent).Members[deriveMethods[proto]]
	if !ok || len(r.Overloads[set].Members) == 0 {
		return ""
	}
	req := r.requirement(iface, deriveMethods[proto])
	if req == 0 {
		return ""
	}
	_, why, _, _ := r.findWitness(t, req, map[EntityID]TypeID{r.iface(iface).SelfParam: t}, r.Entities[n.Ent].Pkg)
	return why
}
