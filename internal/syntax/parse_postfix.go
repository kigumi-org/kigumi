package syntax

import "kigumi/internal/token"

// postfix parses a primary followed by calls, indexing, member access, `?`
// and a record literal, then applies the whitespace-call rule.
func (p *parser) postfix() NodeID {
	e := p.postfixChain(p.primary())
	if e == 0 {
		return 0
	}
	if !p.isWsCallee(e) || !p.startsWsArg() {
		return e
	}
	argTok := p.pos
	arg := p.postfixChain(p.primary())
	call := p.node2(WsCallExpr, p.tree.Nodes[e].Tok, e, p.tree.addList(List, argTok, []NodeID{arg}))
	if p.startsWsArg() {
		p.errorf("whitespace call takes exactly one argument; write `f(g(x))` or `f(g, x)` (§4.2)")
	}
	return call
}

func (p *parser) postfixChain(e NodeID) NodeID {
	for e != 0 {
		switch {
		case p.at(token.LParen):
			open := p.pos
			args := p.exprList()
			e = p.node2(CallExpr, p.tree.Nodes[e].Tok, e, p.tree.addList(List, open, args))
		case p.at(token.LBracket) && !p.tok().HasSpaceBefore():
			var items []NodeID
			tok := p.layoutList(token.LBracket, token.RBracket, func() {
				if p.startsTypeOnly() {
					items = append(items, p.typ())
				} else {
					items = append(items, p.expr())
				}
			})
			e = p.node2(BracketExpr, tok, e, p.tree.addList(List, tok, items))
		case p.at(token.Dot):
			p.advance()
			e = p.node1(MemberExpr, p.memberName(), e)
		case p.at(token.QuestionDot):
			p.advance()
			e = p.node1(OptMemberExpr, p.memberName(), e)
		case p.at(token.Question) && !p.tok().HasSpaceBefore():
			e = p.node1(TryExpr, p.advance(), e)
		case p.at(token.Newline) && p.peekPastNewlines() == token.Dot:
			p.skipNewlines()
		case p.at(token.LBrace) && p.isRecordHead(e) && (!p.noRecordLit || p.looksLikeRecordBody()):
			e = p.recordLit(e)
		default:
			return e
		}
	}
	return e
}

// startsTypeOnly is true at a token that opens a type but never an
// expression, so a bracket list item there is a type argument.
func (p *parser) startsTypeOnly() bool {
	return p.at(token.KwExtern) || p.at(token.KwFn) || p.at(token.KwPure) || p.at(token.KwNoalloc) || p.at(token.Star)
}

func (p *parser) memberName() uint32 {
	if p.at(token.Ident) {
		return p.advance()
	}
	p.errorf("expected member name, found %s", p.describe())
	return p.pos
}

// isRecordHead reports whether e has the shape of a type name: an identifier
// or member path, optionally with `[T]` arguments. Case is not consulted;
// whether the name is a type is decided by name resolution.
func (p *parser) isRecordHead(e NodeID) bool {
	n := p.tree.Nodes[e]
	switch n.Kind {
	case Ident, MemberExpr:
		return true
	case BracketExpr:
		return p.isRecordHead(NodeID(n.Lhs))
	}
	return false
}

// looksLikeRecordBody tells a record literal from a block in a condition:
// a body opening with `name:` or `..` cannot start a block.
func (p *parser) looksLikeRecordBody() bool {
	i := 1
	for p.peek(i) == token.Newline {
		i++
	}
	return p.peek(i) == token.DotDot || p.peek(i) == token.Ident && p.peek(i+1) == token.Colon
}

func (p *parser) recordLit(head NodeID) NodeID {
	var entries []NodeID
	tok := p.layoutList(token.LBrace, token.RBrace, func() {
		if p.at(token.DotDot) {
			dots := p.advance()
			entries = append(entries, p.node1(Spread, dots, p.expr()))
			return
		}
		name := p.expect(token.Ident)
		p.expect(token.Colon)
		p.skipNewlines()
		entries = append(entries, p.node1(FieldInit, name, p.expr()))
	})
	return p.node2(RecordLit, tok, head, p.tree.addList(List, tok, entries))
}

func (p *parser) isWsCallee(e NodeID) bool {
	switch p.tree.Nodes[e].Kind {
	case Ident, MemberExpr:
		return true
	}
	return false
}

// startsWsArg reports whether the current token begins a primary expression
// on the same line, separated by whitespace from the callee.
func (p *parser) startsWsArg() bool {
	t := p.tok()
	if !t.HasSpaceBefore() {
		return false
	}
	switch t.Kind {
	case token.Ident:
		return p.text() != "_"
	case token.Int, token.Float, token.String, token.ByteString, token.Char, token.KwTrue, token.KwFalse,
		token.Dollar:
		return true
	}
	return false
}
