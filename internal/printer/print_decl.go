package printer

import (
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func (p *printer) prefix(id syntax.NodeID) {
	s := p.t.Slots(id)
	if doc := p.t.Slot(id, "doc"); doc != 0 {
		for i := firstDocTok(p.t, doc); i <= doc; i++ {
			if p.t.Toks[i].Kind == token.DocComment {
				p.write(p.tok(i))
				p.newline()
			}
		}
		p.skipTo(doc + 1)
	}
	if attrs := p.t.Slot(id, "attrs"); attrs != 0 {
		p.attrs(syntax.NodeID(attrs))
	}
	if vis := p.t.Slot(id, "vis"); vis != 0 {
		p.visibility(syntax.NodeID(vis))
	}
	if p.t.Kind(id) == syntax.FnDecl && s[2] != 0 {
		p.write(modsText(s[2]) + " ")
	}
}

func (p *printer) attrs(list syntax.NodeID) {
	var impls []syntax.NodeID
	for _, a := range p.t.Children(list) {
		n := p.node(a)
		if p.pathText(syntax.NodeID(n.Lhs)) == "impl" {
			impls = append(impls, p.t.Children(syntax.NodeID(n.Rhs))...)
			continue
		}
		p.write("@" + p.pathText(syntax.NodeID(n.Lhs)))
		if n.Rhs != 0 {
			p.write("(")
			p.exprList(syntax.NodeID(n.Rhs))
			p.write(")")
		}
		p.newline()
	}
	if len(impls) > 0 {
		p.write("@impl(")
		for i, e := range impls {
			if i > 0 {
				p.write(", ")
			}
			p.expr(e)
		}
		p.write(")")
		p.newline()
	}
}

func (p *printer) visibility(id syntax.NodeID) {
	n := p.node(id)
	if n.Lhs == 0 {
		p.write("pub ")
		return
	}
	switch p.tok(n.Lhs) {
	case "self":
		return
	case "in":
		p.write("pub(in " + p.pathText(syntax.NodeID(n.Rhs)) + ") ")
	default:
		p.write("pub(" + p.tok(n.Lhs) + ") ")
	}
}

func (p *printer) pathText(id syntax.NodeID) string {
	s := ""
	for i, tk := range p.t.PathToks(id) {
		if i > 0 {
			s += "."
		}
		s += p.tok(tk)
	}
	return s
}

func (p *printer) fnDecl(id syntax.NodeID) {
	p.prefix(id)
	s := p.t.Slots(id)
	p.write("fn ")
	if s[4] != 0 {
		p.typ(syntax.NodeID(s[4]))
		p.write(".")
	}
	p.write(p.tok(s[5]))
	if s[6] != 0 {
		p.genericParams(syntax.NodeID(s[6]))
	}
	p.params(syntax.NodeID(s[7]))
	if s[8] != 0 {
		p.write(" -> ")
		p.typ(syntax.NodeID(s[8]))
	}
	if s[9] != 0 {
		p.write(" ")
		p.block(syntax.NodeID(s[9]))
	}
}

func (p *printer) genericParams(list syntax.NodeID) {
	p.write("[")
	for i, g := range p.t.Children(list) {
		if i > 0 {
			p.write(", ")
		}
		p.genericParam(g)
	}
	p.write("]")
}

// genericParam prints `T`, `T: A + B`, or `const N: usize`.
func (p *printer) genericParam(g syntax.NodeID) {
	n := p.node(g)
	if n.Lhs&syntax.FlagConst != 0 {
		p.write("const ")
		p.write(p.tok(n.Tok))
		p.write(": ")
		p.typ(syntax.NodeID(n.Rhs))
		return
	}
	p.write(p.tok(n.Tok))
	if n.Rhs != 0 {
		p.write(": ")
		for j, b := range p.t.Children(syntax.NodeID(n.Rhs)) {
			if j > 0 {
				p.write(" + ")
			}
			p.typ(b)
		}
	}
}

func (p *printer) params(list syntax.NodeID) {
	p.write("(")
	for i, prm := range p.t.Children(list) {
		if i > 0 {
			p.write(", ")
		}
		p.param(prm)
	}
	p.write(")")
}

func (p *printer) param(id syntax.NodeID) {
	n := p.node(id)
	s := p.t.Slots(id)
	if s[1]&syntax.FlagVariadic != 0 && s[2] == 0 {
		p.write("...")
		return
	}
	if s[0] != 0 {
		for _, a := range p.t.Children(syntax.NodeID(s[0])) {
			an := p.node(a)
			p.write("@" + p.pathText(syntax.NodeID(an.Lhs)))
			if an.Rhs != 0 {
				p.write("(")
				p.exprList(syntax.NodeID(an.Rhs))
				p.write(")")
			}
			p.write(" ")
		}
	}
	if s[1]&syntax.FlagMut != 0 {
		p.write("mut ")
	}
	if s[1]&syntax.FlagMove != 0 {
		p.write("move ")
	}
	p.write(p.tok(n.Tok))
	if s[2] != 0 {
		p.write(": ")
		if s[1]&syntax.FlagVariadic != 0 {
			p.write("...")
		}
		p.typ(syntax.NodeID(s[2]))
	}
}
