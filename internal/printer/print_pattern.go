package printer

import "kigumi/internal/syntax"

func (p *printer) pattern(id syntax.NodeID) {
	n := p.node(id)
	switch n.Kind {
	case syntax.PatWildcard, syntax.PatBind:
		p.write(p.tok(n.Tok))
	case syntax.PatLit:
		p.expr(syntax.NodeID(n.Lhs))
	case syntax.PatCtor:
		p.write(p.pathText(syntax.NodeID(n.Lhs)))
		if n.Rhs != 0 {
			p.write("(")
			for i, sub := range p.t.Children(syntax.NodeID(n.Rhs)) {
				if i > 0 {
					p.write(", ")
				}
				p.pattern(sub)
			}
			p.write(")")
		}
	case syntax.PatTuple:
		items := p.t.Children(id)
		p.write("(")
		for i, sub := range items {
			if i > 0 {
				p.write(", ")
			}
			p.pattern(sub)
		}
		if len(items) == 1 {
			p.write(",")
		}
		p.write(")")
	case syntax.PatRange:
		p.expr(syntax.NodeID(n.Lhs))
		p.write(p.tok(n.Tok))
		p.expr(syntax.NodeID(n.Rhs))
	case syntax.PatOr:
		for i, alt := range p.t.Children(id) {
			if i > 0 {
				p.write(" | ")
			}
			p.pattern(alt)
		}
	case syntax.PatRecord:
		p.write(p.pathText(syntax.NodeID(n.Lhs)) + " {")
		p.newline()
		p.indent++
		for _, f := range p.t.Children(syntax.NodeID(n.Rhs)) {
			fn := p.node(f)
			p.write(p.tok(fn.Tok))
			if fn.Lhs != 0 {
				p.write(": ")
				p.pattern(syntax.NodeID(fn.Lhs))
			}
			p.newline()
		}
		p.indent--
		p.write("}")
	}
}
