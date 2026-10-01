package syntax

import "kigumi/internal/token"

func (p *parser) parseFile() NodeID {
	var decls []NodeID
	p.skipNewlines()
	for !p.at(token.EOF) {
		start := p.pos
		if d := p.decl(); d != 0 {
			decls = append(decls, d)
		}
		if p.pos == start {
			if p.at(token.RBrace) {
				p.errorf("unmatched `}`")
			} else {
				p.errorf("unexpected %s", p.describe())
			}
			p.recover()
			if p.pos == start {
				p.pos++
			}
		}
		p.endOfStatement()
		p.skipNewlines()
	}
	return p.tree.addList(File, 0, decls)
}

// prefix holds the doc comment, attributes, visibility and modifiers that
// precede a declaration, in any order.
type prefix struct {
	doc   uint32
	attrs []NodeID
	vis   NodeID
	mods  uint32
	start uint32
	seen  bool
}

func (p *parser) parsePrefix() prefix {
	var pf prefix
	pf.start = p.pos
	for {
		switch {
		case p.at(token.DocComment):
			pf.doc = p.advance()
			p.skipNewlines()
		case p.at(token.At):
			pf.attrs = append(pf.attrs, p.attr())
			p.skipNewlines()
		case p.at(token.KwPub):
			v := p.visibility()
			if pf.vis != 0 {
				p.errorAt(p.tree.Span(v), "conflicting visibility modifiers")
			}
			pf.vis = v
		case p.at(token.KwPure):
			tok := p.tok()
			p.advance()
			bit := ModPure
			if p.at(token.Question) && !p.tok().HasSpaceBefore() {
				p.advance()
				bit = ModPureVar
			}
			if pf.mods&(ModPure|ModPureVar) != 0 {
				p.warnAt(tok.Span, "duplicate modifier")
			}
			pf.mods |= bit
		case p.at(token.KwNoalloc), p.at(token.KwAsync), p.at(token.KwUnsafe):
			bit := modBit(p.tok().Kind)
			if pf.mods&bit != 0 {
				p.warnAt(p.tok().Span, "duplicate modifier")
			}
			pf.mods |= bit
			p.advance()
		default:
			pf.seen = p.pos != pf.start
			return pf
		}
	}
}

func modBit(k token.Kind) uint32 {
	switch k {
	case token.KwPure:
		return ModPure
	case token.KwNoalloc:
		return ModNoalloc
	case token.KwAsync:
		return ModAsync
	}
	return ModUnsafe
}

func (p *parser) attr() NodeID {
	at := p.expect(token.At)
	path := p.path()
	var args NodeID
	if p.at(token.LParen) && !p.tok().HasSpaceBefore() {
		open := p.pos
		args = p.tree.addList(List, open, p.exprList())
	}
	return p.node2(Attr, at, path, args)
}

func (p *parser) visibility() NodeID {
	tok := p.expect(token.KwPub)
	if !p.at(token.LParen) || p.tok().HasSpaceBefore() {
		return p.tree.add(Node{Kind: Visibility, Tok: tok})
	}
	p.advance()
	kindTok := p.pos
	var path NodeID
	switch {
	case p.atIdent("self"):
		p.errorf("`pub(self)` is the same as no modifier; remove it (v2.6 B4)")
		p.advance()
	case p.atIdent("module"), p.atIdent("super"):
		p.advance()
	case p.at(token.KwIn):
		p.advance()
		path = p.path()
	default:
		p.errorf("expected `self`, `module`, `super` or `in path` after `pub(`")
	}
	p.expect(token.RParen)
	return p.tree.add(Node{Kind: Visibility, Tok: tok, Lhs: kindTok, Rhs: uint32(path)})
}

func (p *parser) decl() NodeID {
	pf := p.parsePrefix()
	if pf.mods != 0 && !p.at(token.KwFn) && !p.at(token.LBrace) {
		p.errorf("`%s` is only allowed on `fn` declarations (§4.5)", modsString(pf.mods))
	}
	switch {
	case p.atIdent("static") && p.peek(1) == token.KwFn:
		p.errorf("`static fn` does not exist; an associated function is `fn Type.name(...)` without `self` (§5.1)")
		p.advance()
		return p.fnDecl(pf)
	case p.atIdent("impl") && p.peek(1) == token.Ident:
		p.errorf("`impl` blocks do not exist; write `fn Type.name(self)` at top level and use `@impl(Interface)` on the type (§5.1)")
		p.recover()
		return 0
	case p.at(token.KwFn):
		return p.fnDecl(pf)
	case p.at(token.KwType):
		return p.typeDecl(pf)
	case p.at(token.KwInterface):
		return p.interfaceDecl(pf)
	case p.at(token.KwConst):
		return p.constDecl(pf)
	case p.at(token.KwImport):
		return p.importDecl(pf)
	case p.atIdent("test") && p.peek(1) == token.String:
		return p.testDecl(pf)
	case p.at(token.KwExtern) || (p.atIdent("export") && p.peek(1) == token.LParen):
		return p.abiBlock(pf)
	case p.at(token.LBrace) && pf.mods == modBit(token.KwUnsafe) && pf.vis == 0 && len(pf.attrs) == 0:
		return p.node1(ExprStmt, pf.start, p.node1(UnsafeExpr, pf.start, p.block()))
	case p.at(token.LBrace) && pf.mods != 0:
		p.errorf("contract blocks were removed from declarations (v2.6 B2); write `%s` on each fn", modsString(pf.mods))
		return p.contractBlock(pf)
	case pf.seen:
		p.errorf("expected declaration after modifiers, found %s", p.describe())
		p.recover()
		return 0
	}
	return p.stmt()
}

func (p *parser) rejectDeclPrefix(pf prefix, what string) {
	if pf.vis != 0 || pf.mods != 0 || len(pf.attrs) > 0 {
		p.errorAt(p.spanFrom(pf.start), "modifiers are not allowed on %s", what)
	}
}

func (p *parser) constDecl(pf prefix) NodeID {
	tok := p.expect(token.KwConst)
	name := p.expect(token.Ident)
	var typ NodeID
	if p.eat(token.Colon) {
		typ = p.typ()
	}
	p.expect(token.Assign)
	value := p.expr()
	return p.tree.addRec(ConstDecl, tok, []uint32{pf.doc, uint32(p.attrList(pf)), uint32(pf.vis), name, uint32(typ), uint32(value)})
}

func (p *parser) attrList(pf prefix) NodeID {
	if len(pf.attrs) == 0 {
		return 0
	}
	return p.tree.addList(List, p.tree.Nodes[pf.attrs[0]].Tok, pf.attrs)
}
