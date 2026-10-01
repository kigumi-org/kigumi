package syntax

import "kigumi/internal/token"

func (p *parser) block() NodeID {
	if !p.enterDepth() {
		return 0
	}
	defer p.exitDepth()
	var stmts []NodeID
	tok := p.expect(token.LBrace)
	saved := p.noRecordLit
	p.noRecordLit = false
	p.skipNewlines()
	for !p.at(token.RBrace) && !p.at(token.EOF) {
		start := p.pos
		if s := p.stmt(); s != 0 {
			stmts = append(stmts, s)
		}
		if p.pos == start {
			p.errorf("unexpected %s", p.describe())
			p.recover()
		}
		p.endOfStatement()
		p.skipNewlines()
	}
	p.expect(token.RBrace)
	p.noRecordLit = saved
	return p.tree.addList(Block, tok, stmts)
}

func (p *parser) stmt() NodeID {
	switch {
	case p.at(token.KwLet):
		return p.letStmt()
	case p.at(token.KwDefer):
		return p.node1(DeferStmt, p.advance(), p.cleanupBody())
	case p.at(token.KwErrdefer):
		return p.node1(ErrdeferStmt, p.advance(), p.cleanupBody())
	}
	e := p.expr()
	if e == 0 {
		return 0
	}
	if p.at(token.Assign) || p.tok().Kind.IsCompoundAssign() {
		op := p.advance()
		return p.node2(AssignStmt, op, e, p.expr())
	}
	return p.node1(ExprStmt, p.tree.Nodes[e].Tok, e)
}

// cleanupBody is the rest of the line as one statement, or a block.
func (p *parser) cleanupBody() NodeID {
	if p.at(token.LBrace) {
		return p.block()
	}
	return p.stmt()
}

func (p *parser) letStmt() NodeID {
	tok := p.expect(token.KwLet)
	var flags uint32
	if p.eat(token.KwMut) {
		flags |= FlagMut
	}
	pat := p.pattern()
	var typ NodeID
	if p.eat(token.Colon) {
		typ = p.typ()
	}
	p.expect(token.Assign)
	init := p.expr()
	var elseBlock NodeID
	if p.at(token.KwElse) {
		p.advance()
		elseBlock = p.block()
	}
	return p.tree.addRec(LetStmt, tok, []uint32{flags, uint32(pat), uint32(typ), uint32(init), uint32(elseBlock)})
}

func (p *parser) ifExpr() NodeID {
	if !p.enterDepth() {
		return 0
	}
	defer p.exitDepth()
	tok := p.expect(token.KwIf)
	if p.at(token.KwLet) {
		return p.ifLet(tok)
	}
	cond := p.condition()
	then := p.block()
	return p.tree.addRec(IfExpr, tok, []uint32{uint32(cond), uint32(then), uint32(p.elseClause())})
}

// ifLet parses `if let pattern = expr; guard { ... }`: the pattern
// may be refutable, taking the then-branch on a match and the else-branch
// otherwise.
func (p *parser) ifLet(tok uint32) NodeID {
	p.expect(token.KwLet)
	var flags uint32
	if p.eat(token.KwMut) {
		flags |= FlagMut
	}
	pat := p.pattern()
	p.expect(token.Assign)
	init := p.condition()
	var guard NodeID
	if p.eat(token.Semicolon) {
		guard = p.condition()
	}
	then := p.block()
	return p.tree.addRec(IfLetExpr, tok, []uint32{flags, uint32(pat), uint32(init), uint32(guard), uint32(then), uint32(p.elseClause())})
}

func (p *parser) elseClause() NodeID {
	if p.at(token.Newline) && p.peekPastNewlines() == token.KwElse {
		p.skipNewlines()
	}
	if !p.eat(token.KwElse) {
		return 0
	}
	if p.at(token.KwIf) {
		return p.ifExpr()
	}
	return p.block()
}

// condition parses an expression where a bare record literal is not allowed.
func (p *parser) condition() NodeID {
	saved := p.noRecordLit
	p.noRecordLit = true
	e := p.expr()
	p.noRecordLit = saved
	return e
}

func (p *parser) matchExpr() NodeID {
	tok := p.expect(token.KwMatch)
	scrutinee := p.condition()
	var arms []NodeID
	p.layoutList(token.LBrace, token.RBrace, func() {
		pat := p.pattern()
		var guard NodeID
		if p.eat(token.KwIf) {
			guard = p.condition()
		}
		p.expect(token.Arrow)
		p.skipNewlines()
		body := p.expr()
		arms = append(arms, p.tree.addRec(MatchArm, p.tree.Nodes[pat].Tok, []uint32{uint32(pat), uint32(guard), uint32(body)}))
	})
	return p.node2(MatchExpr, tok, scrutinee, p.tree.addList(List, tok, arms))
}

// forExpr parses the three loop forms. A top-level `in` before the body brace
// selects the iteration form.
func (p *parser) forExpr() NodeID {
	tok := p.expect(token.KwFor)
	if p.at(token.LBrace) {
		return p.tree.addRec(ForExpr, tok, []uint32{0, 0, uint32(p.block())})
	}
	if p.hasTopLevelIn() {
		pat := p.pattern()
		p.expect(token.KwIn)
		iter := p.condition()
		return p.tree.addRec(ForExpr, tok, []uint32{uint32(pat), uint32(iter), uint32(p.block())})
	}
	cond := p.condition()
	return p.tree.addRec(ForExpr, tok, []uint32{0, uint32(cond), uint32(p.block())})
}

func (p *parser) hasTopLevelIn() bool {
	depth := 0
	for i := p.pos; int(i) < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case token.LParen, token.LBracket, token.LBrace:
			depth++
		case token.RParen, token.RBracket, token.RBrace:
			depth--
		case token.KwIn:
			if depth == 0 {
				return true
			}
		case token.Newline, token.EOF:
			return false
		}
		if depth < 0 {
			return false
		}
	}
	return false
}
