package printer

import (
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func (p *printer) block(id syntax.NodeID) {
	stmts := p.t.Children(id)
	if len(stmts) == 0 {
		p.write("{}")
		return
	}
	p.write("{")
	p.newline()
	p.indent++
	for i, s := range stmts {
		if i > 0 && p.blankBefore(int(p.firstTok(s))) {
			p.blank()
		}
		p.flushComments(p.firstTok(s))
		p.withIndent(func() { p.stmt(s) })
		p.endLine(s)
	}
	p.indent--
	p.write("}")
}

// endLine finishes a statement line, keeping a trailing comment attached.
func (p *printer) endLine(s syntax.NodeID) {
	if c := p.trailingComment(p.lastTok(s)); c != "" {
		p.write(" " + c)
	}
	p.newline()
}

func (p *printer) lastTok(s syntax.NodeID) uint32 {
	sp := p.t.Span(s)
	last := uint32(0)
	for i, tk := range p.t.Toks {
		// Import path segments are synthetic tokens appended after EOF.
		if tk.Kind == token.EOF {
			break
		}
		if tk.End <= sp.End && tk.Start >= sp.Start {
			last = uint32(i)
		}
	}
	return last
}

func (p *printer) stmt(id syntax.NodeID) {
	n := p.node(id)
	switch n.Kind {
	case syntax.LetStmt:
		s := p.t.Slots(id)
		p.write("let ")
		if s[0]&syntax.FlagMut != 0 {
			p.write("mut ")
		}
		p.pattern(syntax.NodeID(s[1]))
		if s[2] != 0 {
			p.write(": ")
			p.typ(syntax.NodeID(s[2]))
		}
		p.write(" = ")
		p.expr(syntax.NodeID(s[3]))
		if s[4] != 0 {
			p.write(" else ")
			p.block(syntax.NodeID(s[4]))
		}
	case syntax.ExprStmt:
		p.expr(syntax.NodeID(n.Lhs))
	case syntax.AssignStmt:
		p.expr(syntax.NodeID(n.Lhs))
		p.write(" " + p.tok(n.Tok) + " ")
		p.expr(syntax.NodeID(n.Rhs))
	case syntax.DeferStmt:
		p.write("defer ")
		p.cleanup(syntax.NodeID(n.Lhs))
	case syntax.ErrdeferStmt:
		p.write("errdefer ")
		p.cleanup(syntax.NodeID(n.Lhs))
	default:
		p.expr(id)
	}
}

func (p *printer) cleanup(body syntax.NodeID) {
	if p.t.Kind(body) == syntax.Block {
		p.block(body)
		return
	}
	p.stmt(body)
}

func (p *printer) ifExpr(id syntax.NodeID) {
	s := p.t.Slots(id)
	p.write("if ")
	p.expr(syntax.NodeID(s[0]))
	p.write(" ")
	p.block(syntax.NodeID(s[1]))
	p.elseClause(syntax.NodeID(s[2]))
}

func (p *printer) ifLet(id syntax.NodeID) {
	s := p.t.Slots(id)
	p.write("if let ")
	if s[0]&syntax.FlagMut != 0 {
		p.write("mut ")
	}
	p.pattern(syntax.NodeID(s[1]))
	p.write(" = ")
	p.expr(syntax.NodeID(s[2]))
	if s[3] != 0 {
		p.write("; ")
		p.expr(syntax.NodeID(s[3]))
	}
	p.write(" ")
	p.block(syntax.NodeID(s[4]))
	p.elseClause(syntax.NodeID(s[5]))
}

func (p *printer) elseClause(e syntax.NodeID) {
	if e == 0 {
		return
	}
	p.write(" else ")
	if p.t.Kind(e) == syntax.Block {
		p.block(e)
		return
	}
	p.expr(e)
}

func (p *printer) matchExpr(id syntax.NodeID) {
	n := p.node(id)
	p.write("match ")
	p.expr(syntax.NodeID(n.Lhs))
	p.write(" {")
	p.newline()
	p.indent++
	for _, arm := range p.t.Children(syntax.NodeID(n.Rhs)) {
		s := p.t.Slots(arm)
		p.flushComments(p.firstTok(arm))
		p.pattern(syntax.NodeID(s[0]))
		if s[1] != 0 {
			p.write(" if ")
			p.expr(syntax.NodeID(s[1]))
		}
		p.write(" -> ")
		p.expr(syntax.NodeID(s[2]))
		p.newline()
	}
	p.indent--
	p.write("}")
}

func (p *printer) forExpr(id syntax.NodeID) {
	s := p.t.Slots(id)
	p.write("for ")
	if s[0] != 0 {
		p.pattern(syntax.NodeID(s[0]))
		p.write(" in ")
	}
	if s[1] != 0 {
		p.expr(syntax.NodeID(s[1]))
		p.write(" ")
	}
	p.block(syntax.NodeID(s[2]))
}
