package printer

import "kigumi/internal/syntax"

func (p *printer) fnType(id syntax.NodeID) {
	s := p.t.Slots(id)
	if s[1] != 0 {
		p.write("extern(" + p.tok(s[1]) + ") ")
	}
	if s[0] != 0 {
		p.write(modsText(s[0]) + " ")
	}
	p.write("fn(")
	params := p.t.Children(syntax.NodeID(s[2]))
	for i, a := range params {
		if i > 0 {
			p.write(", ")
		}
		p.typ(a)
	}
	if s[3]&syntax.FlagVariadic != 0 {
		if len(params) > 0 {
			p.write(", ")
		}
		p.write("...")
	}
	p.write(") -> ")
	p.typ(syntax.NodeID(s[4]))
}
