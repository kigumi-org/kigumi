package hir

import (
	"fmt"
	"strings"
)

func (d *dumper) place(p *Place) string {
	switch p.Kind {
	case PlaceField:
		return fmt.Sprintf("(.%d)", p.Index)
	case PlaceIndex:
		return "([])"
	}
	return d.name(p.Ent)
}

func (d *dumper) pat(p *Pat) string {
	switch p.Kind {
	case PatWild:
		return "_"
	case PatBind:
		return d.name(p.Ent) + ": " + d.r.TypeString(p.Type)
	case PatLit:
		return literalText(p.Lit.Lit)
	case PatCtor:
		var subs []string
		for _, s := range p.Subs {
			subs = append(subs, d.pat(s))
		}
		return d.name(p.Ent) + "(" + strings.Join(subs, ", ") + ")"
	case PatRecord:
		var fields []string
		for _, f := range p.Fields {
			text := fmt.Sprintf(".%d", f.Index)
			if f.Sub != nil {
				text += ": " + d.pat(f.Sub)
			} else {
				text += " " + d.name(f.Bind)
			}
			fields = append(fields, text)
		}
		return d.name(p.Ent) + " {" + strings.Join(fields, ", ") + "}"
	case PatOr:
		var alts []string
		for _, s := range p.Subs {
			alts = append(alts, d.pat(s))
		}
		return strings.Join(alts, " | ")
	case PatRange:
		op := ".."
		if p.Inclusive {
			op = "..="
		}
		return literalText(p.Lo.Lit) + op + literalText(p.Hi.Lit)
	}
	return p.Kind.String()
}
