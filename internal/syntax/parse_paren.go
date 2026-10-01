package syntax

import "kigumi/internal/token"

// parenOrLambda decides between `()` the unit value, `(expr)`, `(a, b)` a
// tuple literal, and `(params) => body` by looking for `=>` after the
// matching `)`; a comma after the first expression is what commits to a
// tuple. `()` is the Unit literal, the 0-tuple counterpart of
// `(a, b)`.
func (p *parser) parenOrLambda() NodeID {
	if p.parenFollowedByFatArrow() {
		return p.lambda()
	}
	tok := p.advance()
	if p.at(token.RParen) {
		p.advance()
		return p.leaf(UnitLit, tok)
	}
	saved := p.noRecordLit
	p.noRecordLit = false
	p.skipNewlines()
	first := p.expr()
	p.skipNewlines()
	if !p.at(token.Comma) {
		p.expect(token.RParen)
		p.noRecordLit = saved
		return p.node1(Paren, tok, first)
	}
	items := []NodeID{first}
	for p.eat(token.Comma) {
		p.skipNewlines()
		if p.at(token.RParen) {
			break
		}
		items = append(items, p.expr())
		p.skipNewlines()
	}
	p.expect(token.RParen)
	p.noRecordLit = saved
	return p.tree.addList(TupleLit, tok, items)
}

func (p *parser) parenFollowedByFatArrow() bool {
	close := p.closer[p.pos]
	if close == 0 {
		return false
	}
	return int(close)+1 < len(p.toks) && p.toks[close+1].Kind == token.FatArrow
}

func (p *parser) lambda() NodeID {
	var params []NodeID
	tok := p.layoutList(token.LParen, token.RParen, func() {
		name := p.expect(token.Ident)
		var typ NodeID
		if p.eat(token.Colon) {
			typ = p.typ()
		}
		params = append(params, p.tree.addRec(Param, name, []uint32{0, 0, uint32(typ)}))
	})
	p.expect(token.FatArrow)
	p.skipNewlines()
	saved := p.noRecordLit
	p.noRecordLit = false
	body := p.expr()
	p.noRecordLit = saved
	return p.node2(Lambda, tok, p.tree.addList(List, tok, params), body)
}
