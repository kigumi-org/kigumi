package sem

type deriver struct {
	r     *Result
	done  map[deriveRequest]bool
	texts map[PackageID]string
	// protos collects the generated Eq / Hash methods, which need no import.
	protos map[PackageID]string
	// visiting guards derivable against recursive types.
	visiting map[EntityID]bool
}

func (g *deriver) derive(d deriveRequest) {
	if g.done[d] {
		return
	}
	g.done[d] = true
	r := g.r
	info := r.typeDecl(d.decl)
	method := deriveMethods[d.iface]
	if set, ok := info.Members[method]; ok && len(r.Overloads[set].Members) > 0 {
		return
	}
	// Nested Option[Option[_]] is rejected at its use site (cJSONNestedOption);
	// no codec is generated here, so the check still sees the gap on retry.
	if d.iface == "Encode" || d.iface == "Decode" {
		if _, bad := r.jsonNestedOptionField(d.decl, d.iface); bad {
			return
		}
	}
	// A type with a component that has no codec keeps no generated one;
	// the constraint check then names the gap on the second pass.
	for _, fld := range info.Fields {
		if !g.derivable(r.field(fld).Type, d.iface) {
			return
		}
	}
	for _, v := range info.Variants {
		for _, pt := range r.variant(v).Payload {
			if !g.derivable(pt, d.iface) {
				return
			}
		}
	}
	var text string
	switch {
	case isProto(d.iface):
		text = g.proto(d, info)
	case info.Form == FormAdt:
		text = g.adt(d, info)
	default:
		text = g.record(d, info)
	}
	pkg := r.Entities[d.decl].Pkg
	if pkg == 0 {
		pkg = r.pathIndex["std/prelude"]
	}
	if isProto(d.iface) {
		g.protos[pkg] += "\n" + text
	} else {
		g.texts[pkg] += "\n" + text
	}
	for _, fld := range info.Fields {
		g.nested(r.field(fld).Type, d.iface)
	}
	for _, v := range info.Variants {
		for _, pt := range r.variant(v).Payload {
			g.nested(pt, d.iface)
		}
	}
}

func (g *deriver) nested(t TypeID, iface string) {
	n := g.r.Types.Node(t)
	for _, a := range n.Args {
		g.nested(a, iface)
	}
	if n.Elem != 0 {
		g.nested(n.Elem, iface)
	}
	if n.Kind != KNamed || (g.r.Packages[g.r.Entities[n.Ent].Pkg].Std && !isProto(iface)) {
		return
	}
	if form := g.r.typeDecl(n.Ent).Form; form == FormRecord || form == FormAdt || (form == FormResource && !isProto(iface)) {
		g.derive(deriveRequest{decl: n.Ent, iface: iface})
	}
}
