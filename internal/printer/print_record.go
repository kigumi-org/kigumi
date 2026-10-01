package printer

import "kigumi/internal/syntax"

func (p *printer) recordLit(id syntax.NodeID) {
	n := p.node(id)
	p.expr(syntax.NodeID(n.Lhs))
	entries := p.t.Children(syntax.NodeID(n.Rhs))
	if len(entries) == 1 {
		p.write(" { ")
		p.recordEntry(entries[0])
		p.write(" }")
		return
	}
	p.write(" {")
	p.newline()
	p.indent++
	for _, e := range entries {
		p.withIndent(func() { p.recordEntry(e) })
		p.newline()
	}
	p.indent--
	p.write("}")
}

func (p *printer) recordEntry(e syntax.NodeID) {
	n := p.node(e)
	if n.Kind == syntax.Spread {
		p.write("..")
		p.expr(syntax.NodeID(n.Lhs))
		return
	}
	p.write(p.tok(n.Tok) + ": ")
	p.expr(syntax.NodeID(n.Lhs))
}

// tupleLit prints `(a, b, ...)`; a single element keeps its trailing comma
// so it reparses as a tuple and not a grouped expression.
func (p *printer) tupleLit(id syntax.NodeID) {
	items := p.t.Children(id)
	p.write("(")
	p.exprList(id)
	if len(items) == 1 {
		p.write(",")
	}
	p.write(")")
}

func (p *printer) lambda(id syntax.NodeID) {
	n := p.node(id)
	p.params(syntax.NodeID(n.Lhs))
	p.write(" => ")
	p.expr(syntax.NodeID(n.Rhs))
}
