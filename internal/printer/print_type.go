package printer

import (
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func (p *printer) typeDecl(id syntax.NodeID) {
	p.prefix(id)
	s := p.t.Slots(id)
	p.write("type " + p.tok(s[3]))
	if s[4] != 0 {
		p.genericParams(syntax.NodeID(s[4]))
	}
	if s[5] != 0 {
		p.write(" layout(")
		p.layoutArgs(syntax.NodeID(p.node(syntax.NodeID(s[5])).Lhs))
		p.write(")")
	}
	if s[6] == 0 {
		return
	}
	p.write(" =")
	body := syntax.NodeID(s[6])
	switch p.t.Kind(body) {
	case syntax.ResourceBody:
		p.write(" resource ")
		p.fields(body)
	case syntax.RecordBody:
		p.write(" ")
		p.fields(body)
	case syntax.AdtBody:
		p.variants(body)
	case syntax.AliasBody:
		p.write(" ")
		p.typ(syntax.NodeID(p.node(body).Lhs))
	}
}

// layoutArgs prints a `layout(...)` clause's arguments: bare names (`C`,
// `packed`) and `name: value` args (`align: N`) alike.
func (p *printer) layoutArgs(list syntax.NodeID) {
	for i, a := range p.t.Children(list) {
		if i > 0 {
			p.write(", ")
		}
		if p.t.Kind(a) == syntax.FieldInit {
			p.recordEntry(a)
			continue
		}
		p.expr(a)
	}
}

func (p *printer) fields(body syntax.NodeID) {
	fields := p.t.Children(body)
	if len(fields) == 0 {
		p.write("{}")
		return
	}
	p.write("{")
	p.newline()
	p.indent++
	for i, f := range fields {
		s := p.t.Slots(f)
		if i > 0 && (s[3] != 0 || p.t.Slots(fields[i-1])[3] != 0) {
			p.blank()
		}
		p.flushComments(p.firstTok(f))
		if s[0] != 0 {
			p.visibility(syntax.NodeID(s[0]))
		}
		if s[1]&syntax.FlagMut != 0 {
			p.write("mut ")
		}
		p.write(p.tok(p.node(f).Tok) + " ")
		p.typ(syntax.NodeID(s[2]))
		if s[3] != 0 {
			p.write(" ")
			p.metadata(syntax.NodeID(s[3]))
		}
		p.newline()
	}
	p.indent--
	p.write("}")
}

func (p *printer) metadata(meta syntax.NodeID) {
	p.write("{")
	p.newline()
	p.indent++
	for _, item := range p.t.Children(meta) {
		n := p.node(item)
		p.write(p.pathText(syntax.NodeID(n.Lhs)))
		if n.Rhs != 0 {
			p.write("(")
			p.exprList(syntax.NodeID(n.Rhs))
			p.write(")")
		}
		p.newline()
	}
	p.indent--
	p.write("}")
}

func (p *printer) typ(id syntax.NodeID) {
	n := p.node(id)
	switch n.Kind {
	case syntax.TypePath:
		p.write(p.pathText(syntax.NodeID(n.Lhs)))
		if n.Rhs != 0 {
			p.write("[")
			for i, a := range p.t.Children(syntax.NodeID(n.Rhs)) {
				if i > 0 {
					p.write(", ")
				}
				switch p.t.Kind(a) {
				case syntax.GenericParam:
					p.genericParam(a)
				case syntax.IntLit, syntax.BoolLit:
					p.expr(a)
				case syntax.TypeLifetime:
					p.write(p.tok(p.node(a).Tok))
				default:
					p.typ(a)
				}
			}
			p.write("]")
		}
	case syntax.TypeOptional:
		p.typ(syntax.NodeID(n.Lhs))
		p.write("?")
	case syntax.TypeResult:
		p.typ(syntax.NodeID(n.Lhs))
		p.write("!")
	case syntax.TypePtr:
		if n.Lhs&syntax.FlagMut != 0 {
			p.write("*mut ")
		} else {
			p.write("*const ")
		}
		p.typ(syntax.NodeID(n.Rhs))
	case syntax.TypeRef:
		p.write("&")
		if p.t.Toks[n.Tok].Kind == token.Lifetime {
			p.write(p.tok(n.Tok) + " ")
		}
		if n.Lhs&syntax.FlagMut != 0 {
			p.write("mut ")
		}
		p.typ(syntax.NodeID(n.Rhs))
	case syntax.Paren:
		p.write("(")
		p.typ(syntax.NodeID(n.Lhs))
		p.write(")")
	case syntax.TypeTuple:
		items := p.t.Children(id)
		p.write("(")
		for i, a := range items {
			if i > 0 {
				p.write(", ")
			}
			p.typ(a)
		}
		if len(items) == 1 {
			p.write(",")
		}
		p.write(")")
	case syntax.TypeFn:
		p.fnType(id)
	}
}
