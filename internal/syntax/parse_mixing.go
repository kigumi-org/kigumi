package syntax

import "kigumi/internal/token"

// Operator groups for the mixing rule (v2.6 C1): operators from different
// groups may not meet without parentheses, except arithmetic under a
// comparison and a comparison under `&&`/`||`. Within the bit and logic groups
// even different operators need parentheses.
type opGroup uint8

const (
	groupNone opGroup = iota
	groupArith
	groupBit
	groupPipe
	groupCompare
	groupLogic
	groupRange
)

func (p *parser) opGroupOf(tok uint32) opGroup {
	t := p.toks[tok]
	switch t.Kind {
	case token.Plus, token.Minus, token.Star, token.Slash, token.Percent:
		return groupArith
	case token.Shl, token.Shr, token.Amp, token.Caret, token.Pipe:
		return groupBit
	case token.PipeGt:
		return groupPipe
	case token.EqEq, token.NotEq, token.Lt, token.LtEq, token.Gt, token.GtEq, token.KwIs:
		return groupCompare
	case token.AndAnd, token.OrOr:
		return groupLogic
	case token.DotDot, token.DotDotEq:
		return groupRange
	case token.Operator:
		switch t.Text(p.tree.File.Src)[0] {
		case '*', '%', '+', '-':
			return groupArith
		case '<', '>':
			return groupCompare
		case '&', '^':
			return groupBit
		}
	}
	return groupNone
}

// checkMixing reports an error when child, an operand of the operator at
// parentTok, is itself an unparenthesized binary operation that may not be
// mixed with it.
func (p *parser) checkMixing(parentTok uint32, child NodeID) {
	if child == 0 {
		return
	}
	n := p.tree.Nodes[child]
	if n.Kind != Binary && n.Kind != IsExpr {
		return
	}
	pg, cg := p.opGroupOf(parentTok), p.opGroupOf(n.Tok)
	if pg == groupNone || cg == groupNone {
		return
	}
	switch {
	case pg == cg && (pg == groupArith || pg == groupCompare):
		return
	case pg == cg && p.toks[parentTok].Kind == p.toks[n.Tok].Kind &&
		p.tree.TokText(parentTok) == p.tree.TokText(n.Tok):
		return
	case pg == groupCompare && cg == groupArith:
		return
	case pg == groupLogic && cg == groupCompare:
		return
	}
	p.errorAt(p.toks[n.Tok].Span,
		"`%s` and `%s` cannot be mixed without parentheses (v2.6 C1)",
		p.tree.TokText(n.Tok), p.tree.TokText(parentTok))
}

// checkSpacedMinus rejects `f -1`, whose reading as a call or a subtraction
// is ambiguous.
func (p *parser) checkSpacedMinus(opTok uint32, lhs NodeID) {
	op := p.toks[opTok]
	if op.Kind != token.Minus || !op.HasSpaceBefore() || p.tree.Kind(lhs) != Ident {
		return
	}
	if p.tok().HasSpaceBefore() || p.at(token.Newline) {
		return
	}
	p.errorAt(op.Span, "ambiguous `%s -%s`; write `%s - %s` for subtraction or `%s(-%s)` for a call",
		p.tree.TokText(p.tree.Nodes[lhs].Tok), p.describeText(),
		p.tree.TokText(p.tree.Nodes[lhs].Tok), p.describeText(),
		p.tree.TokText(p.tree.Nodes[lhs].Tok), p.describeText())
}

func (p *parser) describeText() string {
	t := p.tok()
	if t.Kind == token.EOF {
		return ""
	}
	return t.Text(p.tree.File.Src)
}
