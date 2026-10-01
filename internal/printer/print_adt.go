package printer

import "kigumi/internal/syntax"

func (p *printer) variants(body syntax.NodeID) {
	vs := p.t.Children(body)
	multi := len(vs) > 3 || p.multiline(body)
	for i, v := range vs {
		switch {
		case multi:
			p.newline()
			p.indent++
			p.write("| ")
			p.indent--
		case i > 0:
			p.write(" | ")
		case len(vs) == 1:
			// A lone variant keeps its bar: without it the declaration
			// reads back as an alias of that name.
			p.write(" | ")
		default:
			p.write(" ")
		}
		n := p.node(v)
		p.write(p.tok(n.Tok))
		if n.Lhs != 0 {
			p.write("(")
			for j, f := range p.t.Children(syntax.NodeID(n.Lhs)) {
				if j > 0 {
					p.write(", ")
				}
				fn := p.node(f)
				if fn.Lhs&syntax.FlagNamed != 0 {
					p.write(p.tok(fn.Tok) + ": ")
				}
				p.typ(syntax.NodeID(fn.Rhs))
			}
			p.write(")")
		}
	}
}
