package syntax

import "kigumi/internal/token"

func (p *parser) optionalOperand() NodeID {
	switch p.tok().Kind {
	case token.Newline, token.Semicolon, token.RBrace, token.RParen, token.Comma, token.EOF:
		return 0
	}
	return p.expr()
}

func (p *parser) contractExpr() NodeID {
	tok := p.pos
	var mods uint32
	for p.at(token.KwPure) || p.at(token.KwNoalloc) {
		mods |= modBit(p.tok().Kind)
		p.advance()
	}
	if !p.at(token.LBrace) {
		p.errorf("expected `{` after contract modifiers in expression position")
		return 0
	}
	return p.nodeFlag(ContractExpr, tok, mods, p.block())
}

func (p *parser) identPrimary() NodeID {
	if p.atIdent("nil") {
		p.errorf("`nil` does not exist; the absent value is `None` (v2.6 B3)")
	}
	if p.atIdent("asm") && p.peek(1) == token.LBrace && p.startsAsmBlock() {
		return p.asmBlock()
	}
	if p.atIdent("allocator") && p.startsAllocatorBlock() {
		tok := p.advance()
		scope := p.condition()
		return p.node2(AllocatorExpr, tok, scope, p.block())
	}
	return p.leaf(Ident, p.advance())
}

// startsAllocatorBlock distinguishes `allocator handle.scope() { ... }` from a
// variable that happens to be called allocator.
func (p *parser) startsAllocatorBlock() bool {
	if int(p.pos)+1 >= len(p.toks) || !p.toks[p.pos+1].HasSpaceBefore() {
		return false
	}
	switch p.peek(1) {
	case token.Ident, token.LParen, token.String:
		return true
	}
	return false
}
