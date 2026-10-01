package syntax

import "kigumi/internal/token"

func (p *parser) typeDecl(pf prefix) NodeID {
	tok := p.expect(token.KwType)
	name := p.expect(token.Ident)
	var generics, layout, body NodeID
	if p.at(token.LBracket) {
		generics = p.genericParams()
	}
	if p.atIdent("layout") && p.peek(1) == token.LParen {
		layout = p.layoutClause()
	}
	if p.eat(token.Assign) {
		if p.at(token.Newline) && p.startsAdtAfterNewlines() {
			p.skipNewlines()
		}
		body = p.typeBody()
	}
	return p.tree.addRec(TypeDecl, tok, []uint32{
		pf.doc, uint32(p.attrList(pf)), uint32(pf.vis), name, uint32(generics), uint32(layout), uint32(body),
	})
}

// startsAdtAfterNewlines mirrors the ADT detection in typeBody so `type T =`
// can drop to the first variant without a leading `|` (request: adt-leading-bar).
func (p *parser) startsAdtAfterNewlines() bool {
	i := p.pos + 1
	for int(i) < len(p.toks) && p.toks[i].Kind == token.Newline {
		i++
	}
	if int(i) >= len(p.toks) {
		return false
	}
	switch p.toks[i].Kind {
	case token.Pipe:
		return true
	case token.Ident:
		j := i + 1
		if int(j) < len(p.toks) && p.toks[j].Kind == token.LParen {
			return true
		}
		for int(j) < len(p.toks) && p.toks[j].Kind == token.Newline {
			j++
		}
		return int(j) < len(p.toks) && p.toks[j].Kind == token.Pipe
	}
	return false
}

func (p *parser) layoutClause() NodeID {
	tok := p.advance()
	var args []NodeID
	p.layoutList(token.LParen, token.RParen, func() {
		if p.at(token.Ident) && p.peek(1) == token.Colon {
			name := p.advance()
			p.advance()
			args = append(args, p.node1(FieldInit, name, p.expr()))
			return
		}
		args = append(args, p.expr())
	})
	return p.node1(LayoutClause, tok, p.tree.addList(List, tok, args))
}

func (p *parser) typeBody() NodeID {
	switch {
	case p.atIdent("resource") && p.peek(1) == token.LBrace:
		tok := p.advance()
		return p.fieldList(ResourceBody, tok)
	case p.at(token.LBrace):
		return p.fieldList(RecordBody, p.pos)
	case p.at(token.Ident) && (p.peek(1) == token.LParen || p.peek(1) == token.Pipe ||
		(p.peek(1) == token.Newline && p.peekPastNewlines() == token.Pipe)):
		return p.adtBody()
	case p.at(token.Pipe):
		p.advance()
		return p.adtBody()
	}
	return p.node1(AliasBody, p.pos, p.typ())
}

func (p *parser) fieldList(kind NodeKind, tok uint32) NodeID {
	var fields []NodeID
	p.layoutList(token.LBrace, token.RBrace, func() {
		fields = append(fields, p.field())
	})
	return p.tree.addList(kind, tok, fields)
}

// field parses `pub? mut? name Type { metadata }?`. A `fn` here is the
// forbidden in-body method and gets a pointed error.
func (p *parser) field() NodeID {
	var vis NodeID
	if p.at(token.KwPub) {
		vis = p.visibility()
	}
	var flags uint32
	if p.eat(token.KwMut) {
		flags |= FlagMut
	}
	if p.at(token.KwFn) {
		p.errorf("methods cannot be declared inside a type body; write `fn Type.name(self)` at top level (§5.1)")
		p.recoverList(token.RBrace)
		return p.tree.addRec(Field, p.pos, []uint32{0, 0, 0, 0})
	}
	name := p.expect(token.Ident)
	if p.at(token.Colon) {
		p.errorf("field declarations use `name Type` without `:`")
		p.advance()
	}
	typ := p.typ()
	var meta NodeID
	if p.at(token.LBrace) {
		meta = p.metadata()
	}
	return p.tree.addRec(Field, name, []uint32{uint32(vis), flags, uint32(typ), uint32(meta)})
}

func (p *parser) metadata() NodeID {
	var items []NodeID
	tok := p.layoutList(token.LBrace, token.RBrace, func() {
		path := p.path()
		var args NodeID
		if p.at(token.LParen) {
			open := p.pos
			args = p.tree.addList(List, open, p.exprList())
		}
		items = append(items, p.node2(MetaItem, p.tree.Nodes[path].Tok, path, args))
	})
	return p.tree.addList(Metadata, tok, items)
}

func (p *parser) adtBody() NodeID {
	tok := p.pos
	var variants []NodeID
	for {
		variants = append(variants, p.variant())
		if p.at(token.Newline) && p.peekPastNewlines() == token.Pipe {
			p.skipNewlines()
		}
		if !p.eat(token.Pipe) {
			break
		}
		p.skipNewlines()
	}
	return p.tree.addList(AdtBody, tok, variants)
}

func (p *parser) variant() NodeID {
	name := p.expect(token.Ident)
	var fields NodeID
	if p.at(token.LParen) {
		var items []NodeID
		tok := p.layoutList(token.LParen, token.RParen, func() {
			if p.at(token.Ident) && p.peek(1) == token.Colon {
				fname := p.advance()
				p.advance()
				items = append(items, p.nodeFlag(VariantField, fname, FlagNamed, p.typ()))
				return
			}
			start := p.pos
			items = append(items, p.nodeFlag(VariantField, start, 0, p.typ()))
		})
		fields = p.tree.addList(List, tok, items)
	}
	return p.node1(Variant, name, fields)
}
