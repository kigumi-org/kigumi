package syntax

import "kigumi/internal/token"

// pattern parses `alt | alt | ...`; a single alternative is returned as is.
func (p *parser) pattern() NodeID {
	if !p.enterDepth() {
		return 0
	}
	defer p.exitDepth()
	first := p.pos
	alt := p.patternAtom()
	if !p.at(token.Pipe) {
		return alt
	}
	items := []NodeID{alt}
	for p.eat(token.Pipe) {
		items = append(items, p.patternAtom())
	}
	return p.tree.addList(PatOr, first, items)
}

func (p *parser) patternAtom() NodeID {
	switch {
	case p.at(token.Ident) && p.text() == "_":
		return p.leaf(PatWildcard, p.advance())
	case p.at(token.Int):
		tok := p.pos
		lit := p.literal()
		if op, ok := p.rangeOp(); ok {
			return p.node2(PatRange, op, lit, p.rangeBound())
		}
		return p.node1(PatLit, tok, lit)
	case p.at(token.Float), p.at(token.String), p.at(token.ByteString), p.at(token.Char),
		p.at(token.KwTrue), p.at(token.KwFalse):
		tok := p.pos
		return p.node1(PatLit, tok, p.literal())
	case p.at(token.Minus) && p.peek(1) == token.Int:
		tok := p.advance()
		lit := p.node1(Unary, tok, p.literal())
		if op, ok := p.rangeOp(); ok {
			return p.node2(PatRange, op, lit, p.rangeBound())
		}
		return p.node1(PatLit, tok, lit)
	case p.at(token.Minus) && p.peek(1) == token.Float:
		tok := p.advance()
		return p.node1(PatLit, tok, p.node1(Unary, tok, p.literal()))
	case p.at(token.Ident):
		return p.pathPattern()
	case p.at(token.LParen):
		return p.parenOrTuplePattern()
	}
	p.errorf("expected pattern, found %s", p.describe())
	return 0
}

// rangeOp reports whether the current token opens a range pattern,
// consuming `..` or `..=`.
func (p *parser) rangeOp() (uint32, bool) {
	if p.at(token.DotDot) || p.at(token.DotDotEq) {
		return p.advance(), true
	}
	return 0, false
}

// rangeBound parses one endpoint of a range pattern: an integer
// literal or a named constant, checked further by sem.
func (p *parser) rangeBound() NodeID {
	switch {
	case p.at(token.Minus) && p.peek(1) == token.Int:
		tok := p.advance()
		return p.node1(Unary, tok, p.leaf(IntLit, p.advance()))
	case p.at(token.Int):
		return p.leaf(IntLit, p.advance())
	case p.at(token.Ident):
		return p.leaf(Ident, p.advance())
	}
	p.errorf("expected integer literal or constant, found %s", p.describe())
	return 0
}

// parenOrTuplePattern parses `(pat)`, a grouped pattern, and `(a, b)`, a
// tuple pattern: a comma after the first element commits to a tuple, the
// same rule as tuple types and literals.
func (p *parser) parenOrTuplePattern() NodeID {
	tok := p.advance()
	saved := p.noRecordLit
	p.noRecordLit = false
	p.skipNewlines()
	first := p.pattern()
	p.skipNewlines()
	if !p.at(token.Comma) {
		p.expect(token.RParen)
		p.noRecordLit = saved
		return first
	}
	items := []NodeID{first}
	for p.eat(token.Comma) {
		p.skipNewlines()
		if p.at(token.RParen) {
			break
		}
		items = append(items, p.pattern())
		p.skipNewlines()
	}
	p.expect(token.RParen)
	p.noRecordLit = saved
	return p.tree.addList(PatTuple, tok, items)
}

// Identifiers starting with an upper-case letter are constructors;
// others bind.
func (p *parser) pathPattern() NodeID {
	first := p.pos
	if p.text() == "nil" {
		p.errorf("`nil` does not exist; the absent value is `None` (v2.6 B3)")
	}
	isCtor := p.isUpper(first) || (p.peek(1) == token.Dot)
	// An upper-case named constant directly followed by a range op is a
	// range-pattern low bound, not a bare constructor pattern.
	if p.peek(1) == token.DotDot || p.peek(1) == token.DotDotEq {
		isCtor = false
	}
	if !isCtor {
		tok := p.advance()
		if p.at(token.LParen) && !p.tok().HasSpaceBefore() {
			p.errorAt(p.toks[tok].Span, "constructor names start with an upper-case letter; `%s` is read as a binding", p.toks[tok].Text(p.tree.File.Src))
		}
		if op, ok := p.rangeOp(); ok {
			return p.node2(PatRange, op, p.leaf(Ident, tok), p.rangeBound())
		}
		return p.nodeFlag(PatBind, tok, 0, 0)
	}
	path := p.path()
	switch {
	case p.at(token.LParen) && !p.tok().HasSpaceBefore():
		var items []NodeID
		// Inside the parentheses a `{` can only start a record pattern.
		saved := p.noRecordLit
		p.noRecordLit = false
		tok := p.layoutList(token.LParen, token.RParen, func() {
			items = append(items, p.pattern())
		})
		p.noRecordLit = saved
		return p.node2(PatCtor, first, path, p.tree.addList(List, tok, items))
	case p.at(token.LBrace) && !p.noRecordLit:
		var items []NodeID
		tok := p.layoutList(token.LBrace, token.RBrace, func() {
			name := p.expect(token.Ident)
			var sub NodeID
			if p.eat(token.Colon) {
				sub = p.pattern()
			}
			items = append(items, p.node1(PatField, name, sub))
		})
		return p.node2(PatRecord, first, path, p.tree.addList(List, tok, items))
	}
	return p.node2(PatCtor, first, path, 0)
}

func (p *parser) literal() NodeID {
	switch p.tok().Kind {
	case token.Int:
		return p.leaf(IntLit, p.advance())
	case token.Float:
		return p.leaf(FloatLit, p.advance())
	case token.Char:
		return p.leaf(CharLit, p.advance())
	case token.KwTrue, token.KwFalse:
		return p.leaf(BoolLit, p.advance())
	case token.String:
		return p.stringLit()
	case token.ByteString:
		return p.byteStringLit()
	}
	p.errorf("expected literal, found %s", p.describe())
	return 0
}
