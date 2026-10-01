package sem

import (
	"fmt"
	"strings"
)

func (g *deriver) binder(d deriveRequest, info *TypeDeclInfo) string {
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
		ps = append(ps, g.r.Entities[p].Name+": json."+d.iface)
	}
	return name + "[" + strings.Join(ps, ", ") + "]"
}

func (g *deriver) instance(d deriveRequest, info *TypeDeclInfo) string {
	name := g.r.Entities[d.decl].Name
	if len(info.Params) == 0 {
		return name
	}
	var ps []string
	for _, p := range info.Params {
		ps = append(ps, g.r.Entities[p].Name)
	}
	return name + "[" + strings.Join(ps, ", ") + "]"
}

func (g *deriver) jsonKey(fld EntityID) string {
	for _, m := range g.r.field(fld).Metadata {
		if g.r.Entities[m.Head].Name == "name" && len(m.Args) == 1 {
			return m.Args[0].Str
		}
	}
	return g.r.Entities[fld].Name
}

func (g *deriver) omitNone(fld EntityID) bool {
	for _, m := range g.r.field(fld).Metadata {
		if g.r.Entities[m.Head].Name == "omitNone" {
			return true
		}
	}
	return false
}

func (g *deriver) record(d deriveRequest, info *TypeDeclInfo) string {
	var sb strings.Builder
	name := g.r.Entities[d.decl].Name
	if d.iface == "Encode" {
		fmt.Fprintf(&sb, "fn %s.toJson(self) -> json.Value {\n    let mut f = json.Fields.new()\n", g.binder(d, info))
		for _, fld := range info.Fields {
			fname := g.r.Entities[fld].Name
			if g.omitNone(fld) {
				fmt.Fprintf(&sb, "    if self.%s is Some(_) {\n        f.put(%q, self.%s.toJson())\n    }\n", fname, g.jsonKey(fld), fname)
			} else {
				fmt.Fprintf(&sb, "    f.put(%q, self.%s.toJson())\n", g.jsonKey(fld), fname)
			}
		}
		sb.WriteString("    f.finish()\n}\n")
		return sb.String()
	}
	fmt.Fprintf(&sb, "fn %s.fromJson(value: json.Value) -> %s! {\n    let mut o = json.Record.expect(value, %q)?\n    Ok(%s {", g.binder(d, info), g.instance(d, info), name, name)
	for i, fld := range info.Fields {
		if i > 0 {
			sb.WriteString(",")
		}
		reader := "field"
		if _, ok := g.r.Types.IsOption(g.r.field(fld).Type); ok {
			reader = "optional"
		}
		fmt.Fprintf(&sb, " %s: o.%s(%q)?", g.r.Entities[fld].Name, reader, g.jsonKey(fld))
	}
	sb.WriteString(" })\n}\n")
	return sb.String()
}

func (g *deriver) adt(d deriveRequest, info *TypeDeclInfo) string {
	var sb strings.Builder
	name := g.r.Entities[d.decl].Name
	if d.iface == "Encode" {
		fmt.Fprintf(&sb, "fn %s.toJson(self) -> json.Value {\n    match self {\n", g.binder(d, info))
		for _, v := range info.Variants {
			vname := g.r.Entities[v].Name
			n := len(g.r.variant(v).Payload)
			if n == 0 {
				fmt.Fprintf(&sb, "        %s.%s -> json.tag(%q)\n", name, vname, vname)
				continue
			}
			var binds, encoded []string
			for i := range n {
				binds = append(binds, fmt.Sprintf("p%d", i))
				encoded = append(encoded, fmt.Sprintf("p%d.toJson()", i))
			}
			fmt.Fprintf(&sb, "        %s.%s(%s) -> json.tagged(%q, Array.of(%s))\n", name, vname, strings.Join(binds, ", "), vname, strings.Join(encoded, ", "))
		}
		sb.WriteString("    }\n}\n")
		return sb.String()
	}
	fmt.Fprintf(&sb, "fn %s.fromJson(value: json.Value) -> %s! {\n    let mut t = json.Tagged.expect(value, %q)?\n", g.binder(d, info), g.instance(d, info), name)
	for _, v := range info.Variants {
		vname := g.r.Entities[v].Name
		n := len(g.r.variant(v).Payload)
		if n == 0 {
			fmt.Fprintf(&sb, "    if t.name == %q {\n        t.arity(0)?\n        return Ok(%s.%s)\n    }\n", vname, name, vname)
			continue
		}
		var reads []string
		for range n {
			reads = append(reads, "t.next()?")
		}
		fmt.Fprintf(&sb, "    if t.name == %q {\n        t.arity(%d)?\n        return Ok(%s.%s(%s))\n    }\n", vname, n, name, vname, strings.Join(reads, ", "))
	}
	sb.WriteString("    t.unknown()\n}\n")
	return sb.String()
}
