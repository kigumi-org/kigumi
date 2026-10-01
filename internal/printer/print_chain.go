package printer

import (
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// binary prints operators with single spaces; a chain of the same leading
// operator (`|>`, `&&`, `||`) that spans lines in the source, or whose left
// side is itself the same chain, goes one operand per line with the
// operator leading.
func (p *printer) binary(id syntax.NodeID) {
	n := p.node(id)
	if isLeadingOp(p.t.Toks[n.Tok].Kind) && p.multilineChain(id) {
		p.opChain(id)
		return
	}
	p.expr(syntax.NodeID(n.Lhs))
	switch p.t.Toks[n.Tok].Kind {
	case token.DotDot, token.DotDotEq:
		p.write(p.tok(n.Tok))
	default:
		p.write(" " + p.tok(n.Tok) + " ")
	}
	p.expr(syntax.NodeID(n.Rhs))
}

// isLeadingOp reports whether k's canonical multiline form leads with the
// operator rather than trailing it.
func isLeadingOp(k token.Kind) bool {
	switch k {
	case token.PipeGt, token.AndAnd, token.OrOr:
		return true
	}
	return false
}

// multilineChain reports whether the chain rooted at id actually breaks
// across lines in the source anywhere along its same-operator links — not
// merely that it has 3+ terms, which a same-operator LHS also indicates.
func (p *printer) multilineChain(id syntax.NodeID) bool {
	n := p.node(id)
	if p.opSplitsLine(n.Tok) {
		return true
	}
	lhs := syntax.NodeID(n.Lhs)
	if p.t.Kind(lhs) == syntax.Binary && p.t.Toks[p.node(lhs).Tok].Kind == p.t.Toks[n.Tok].Kind {
		return p.multilineChain(lhs)
	}
	return false
}

// opSplitsLine reports whether the source actually breaks the line at the
// operator itself (leading or trailing form), rather than merely containing
// a multi-line RHS such as a match/if block or a trailing-comma call — those
// span multiple lines without the operator marking a chain split.
func (p *printer) opSplitsLine(opTok uint32) bool {
	for i := int(opTok) - 1; i >= 0; i-- {
		switch p.t.Toks[i].Kind {
		case token.Newline:
			return true
		case token.Comment, token.BlockComment, token.DocComment:
			continue
		}
		break
	}
	for i := int(opTok) + 1; i < len(p.t.Toks); i++ {
		switch p.t.Toks[i].Kind {
		case token.Newline:
			return true
		case token.Comment, token.BlockComment, token.DocComment:
			continue
		}
		break
	}
	return false
}

func (p *printer) opChain(id syntax.NodeID) {
	n := p.node(id)
	lhs := syntax.NodeID(n.Lhs)
	opKind := p.t.Toks[n.Tok].Kind
	if p.t.Kind(lhs) == syntax.Binary && p.t.Toks[p.node(lhs).Tok].Kind == opKind {
		p.opChain(lhs)
	} else {
		p.expr(lhs)
	}
	p.newline()
	p.indent++
	p.write(p.tok(n.Tok) + " ")
	p.indent--
	p.expr(syntax.NodeID(n.Rhs))
}
