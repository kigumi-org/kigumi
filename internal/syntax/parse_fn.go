package syntax

import "kigumi/internal/token"

func (p *parser) fnDecl(pf prefix) NodeID {
	tok := p.expect(token.KwFn)
	recv, name := p.fnName()
	var generics NodeID
	if p.at(token.LBracket) {
		generics = p.genericParams()
	}
	params := p.params()
	var ret NodeID
	if p.eat(token.Arrow) {
		ret = p.typ()
	}
	var body NodeID
	if p.at(token.LBrace) {
		body = p.block()
	}
	return p.tree.addRec(FnDecl, tok, []uint32{
		pf.doc, uint32(p.attrList(pf)), pf.mods, uint32(pf.vis), uint32(recv), name,
		uint32(generics), uint32(params), uint32(ret), uint32(body),
	})
}

// fnName parses `name`, `Type.name`, `Type[T].name` or `Type.+^`. A `[` after
// the first identifier is a receiver binder only if `.` follows the `]`,
// disambiguating it from the method's own type parameters: `fn Array[T].map[U](...)`.
func (p *parser) fnName() (recv NodeID, name uint32) {
	first := p.expect(token.Ident)
	if p.at(token.LBracket) && p.bracketFollowedByDot() {
		// Receiver binders may carry method-level bounds: `Array[T: Eq].contains`.
		recv = p.node2(TypePath, first, p.singlePath(first), p.genericParams())
	} else if p.at(token.Dot) {
		recv = p.node2(TypePath, first, p.singlePath(first), 0)
	} else {
		return 0, first
	}
	p.expect(token.Dot)
	if p.at(token.Ident) || isOperatorToken(p.tok().Kind) {
		return recv, p.advance()
	}
	p.errorf("expected method name or operator after `.`, found %s", p.describe())
	return recv, p.pos
}

func (p *parser) singlePath(tok uint32) NodeID {
	path := p.tree.add(Node{Kind: Path, Tok: tok, Lhs: uint32(len(p.tree.Extra)), Rhs: uint32(len(p.tree.Extra)) + 1})
	p.tree.Extra = append(p.tree.Extra, tok)
	return path
}

func (p *parser) bracketFollowedByDot() bool {
	depth := 0
	for i := p.pos; int(i) < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case token.LBracket:
			depth++
		case token.RBracket:
			depth--
			if depth == 0 {
				return int(i)+1 < len(p.toks) && p.toks[i+1].Kind == token.Dot
			}
		case token.Newline, token.EOF, token.LBrace:
			return false
		}
	}
	return false
}

func isOperatorToken(k token.Kind) bool {
	switch k {
	case token.Operator, token.Plus, token.Minus, token.Star, token.Slash, token.Percent,
		token.Amp, token.Pipe, token.Caret, token.Shl, token.Shr, token.Bang, token.Tilde,
		token.EqEq, token.NotEq, token.Lt, token.LtEq, token.Gt, token.GtEq:
		return true
	}
	return false
}

func (p *parser) genericParams() NodeID {
	var items []NodeID
	tok := p.layoutList(token.LBracket, token.RBracket, func() {
		if p.at(token.Lifetime) {
			items = append(items, p.nodeFlag(GenericParam, p.advance(), FlagLifetime, 0))
			return
		}
		if p.at(token.KwConst) {
			p.advance()
			name := p.expect(token.Ident)
			p.expect(token.Colon)
			items = append(items, p.nodeFlag(GenericParam, name, FlagConst, p.typ()))
			return
		}
		name := p.expect(token.Ident)
		var bounds NodeID
		if p.eat(token.Colon) {
			var types []NodeID
			types = append(types, p.typ())
			for p.at(token.Plus) {
				p.advance()
				types = append(types, p.typ())
			}
			bounds = p.tree.addList(List, name, types)
		}
		items = append(items, p.nodeFlag(GenericParam, name, 0, bounds))
	})
	return p.tree.addList(List, tok, items)
}

func (p *parser) params() NodeID {
	var items []NodeID
	tok := p.layoutList(token.LParen, token.RParen, func() {
		items = append(items, p.param())
	})
	return p.tree.addList(List, tok, items)
}

func (p *parser) param() NodeID {
	var attrs []NodeID
	for p.at(token.At) {
		attrs = append(attrs, p.attr())
	}
	var flags uint32
	if p.at(token.Ellipsis) {
		tok := p.advance()
		return p.tree.addRec(Param, tok, []uint32{0, FlagVariadic, 0})
	}
	if p.eat(token.KwMut) {
		flags |= FlagMut
	} else if p.eat(token.KwMove) {
		flags |= FlagMove
	}
	if p.atIdent("self") {
		tok := p.advance()
		if len(attrs) > 0 {
			p.errorAt(p.tree.Span(attrs[0]), "attributes cannot be applied to `self`")
		}
		return p.tree.addRec(Param, tok, []uint32{0, flags | FlagSelf, 0})
	}
	if flags&FlagMove != 0 {
		p.errorf("`move` is only allowed on `self`")
	}
	name := p.expect(token.Ident)
	p.expect(token.Colon)
	if p.eat(token.Ellipsis) {
		flags |= FlagVariadic
	}
	typ := p.typ()
	var attrList NodeID
	if len(attrs) > 0 {
		attrList = p.tree.addList(List, p.tree.Nodes[attrs[0]].Tok, attrs)
	}
	return p.tree.addRec(Param, name, []uint32{uint32(attrList), flags, uint32(typ)})
}

func (p *parser) interfaceDecl(pf prefix) NodeID {
	tok := p.expect(token.KwInterface)
	name := p.expect(token.Ident)
	var generics NodeID
	if p.at(token.LBracket) {
		generics = p.genericParams()
	}
	members := p.declBlock()
	return p.tree.addRec(InterfaceDecl, tok, []uint32{
		pf.doc, uint32(p.attrList(pf)), uint32(pf.vis), name, uint32(generics), uint32(members),
	})
}
