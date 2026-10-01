package printer

import "kigumi/internal/syntax"

func (p *printer) constDecl(id syntax.NodeID) {
	p.prefix(id)
	s := p.t.Slots(id)
	p.write("const " + p.tok(s[3]))
	if s[4] != 0 {
		p.write(": ")
		p.typ(syntax.NodeID(s[4]))
	}
	p.write(" = ")
	p.expr(syntax.NodeID(s[5]))
}

func (p *printer) importDecl(id syntax.NodeID) {
	s := p.t.Slots(id)
	if s[0] != 0 {
		p.visibility(syntax.NodeID(s[0]))
	}
	p.write("import ")
	binding := syntax.NodeID(s[1])
	if p.t.Kind(binding) == syntax.List {
		names := p.sortedNames(p.t.Children(binding))
		p.write("{")
		for i, nm := range names {
			if i > 0 {
				p.write(", ")
			}
			if p.t.Kind(nm) == syntax.ImportAlias {
				n := p.node(nm)
				p.write(p.tok(p.node(syntax.NodeID(n.Lhs)).Tok) + " as " + p.tok(p.node(syntax.NodeID(n.Rhs)).Tok))
				continue
			}
			p.write(p.tok(p.node(nm).Tok))
		}
		p.write("}")
	} else {
		p.write(p.tok(p.node(binding).Tok))
	}
	p.write(" from ")
	for i, tk := range p.t.PathToks(syntax.NodeID(s[2])) {
		if i > 0 {
			p.write("/")
		}
		p.write(p.tok(tk))
	}
}

func (p *printer) interfaceDecl(id syntax.NodeID) {
	p.prefix(id)
	s := p.t.Slots(id)
	p.write("interface " + p.tok(s[3]))
	if s[4] != 0 {
		p.genericParams(syntax.NodeID(s[4]))
	}
	p.write(" ")
	p.declList(syntax.NodeID(s[5]))
}

func (p *printer) abiBlock(id syntax.NodeID) {
	n := p.node(id)
	p.write(p.tok(n.Tok) + "(")
	for i, a := range p.t.Children(syntax.NodeID(n.Lhs)) {
		if i > 0 {
			p.write(", ")
		}
		p.write(p.tok(p.node(a).Tok))
	}
	p.write(") ")
	p.declList(syntax.NodeID(n.Rhs))
}
