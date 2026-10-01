package printer

import "kigumi/internal/syntax"

// asmBlock prints `asm { ... }` one entry per line in source order.
func (p *printer) asmBlock(id syntax.NodeID) {
	lines := p.t.Children(syntax.NodeID(p.t.Nodes[id].Lhs))
	if len(lines) == 0 {
		p.write("asm {}")
		return
	}
	p.write("asm {")
	p.newline()
	p.indent++
	for i, l := range lines {
		if i > 0 && p.blankBefore(int(p.firstTok(l))) {
			p.blank()
		}
		p.flushComments(p.firstTok(l))
		p.withIndent(func() { p.asmLine(l) })
		p.endLine(l)
	}
	p.indent--
	p.write("}")
}

func (p *printer) asmLine(l syntax.NodeID) {
	n := p.t.Nodes[l]
	switch n.Kind {
	case syntax.AsmTemplate:
		p.expr(syntax.NodeID(n.Lhs))
	case syntax.AsmOperand:
		s := p.t.Slots(l)
		if s[0] != 0 {
			p.write(p.tok(s[0]) + ": ")
		}
		p.write(p.tok(s[1]))
		if s[2] != 0 {
			p.write("(" + p.tok(s[2]) + ")")
		}
		if s[3] != 0 {
			p.write(" ")
			p.expr(syntax.NodeID(s[3]))
		}
	case syntax.AsmClobber, syntax.AsmOptions:
		p.write(p.tok(n.Tok))
		for i, it := range p.t.Children(syntax.NodeID(n.Lhs)) {
			if i == 0 {
				p.write(" ")
			} else {
				p.write(", ")
			}
			if p.t.Kind(it) == syntax.AsmAbi {
				p.write("abi(" + p.tok(p.t.Nodes[it].Tok) + ")")
			} else {
				p.write(p.tok(p.t.Nodes[it].Tok))
			}
		}
	}
}
