package syntax

import (
	"fmt"

	"kigumi/internal/token"
)

func sprintf(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// layoutList parses `open item (sep item)* sep? close` where sep is a comma
// or a newline. items is called once per entry.
func (p *parser) layoutList(open, close token.Kind, item func()) uint32 {
	tok := p.expect(open)
	p.skipNewlines()
	for !p.at(close) && !p.at(token.EOF) {
		start, errs := p.pos, p.bag.Len()
		item()
		if p.pos == start {
			p.errorf("unexpected %s in list", p.describe())
			p.recoverList(close)
			if p.at(token.EOF) {
				break
			}
		}
		// After a broken entry, a new line that starts a declaration means the
		// closer is missing; stop here instead of eating the rest of the file.
		if p.bag.Len() > errs && p.at(token.Newline) && startsDecl(p.peekPastNewlines()) {
			break
		}
		if !p.separator(close) {
			break
		}
		// A stray closer of another kind (`}` inside `(...)`) makes neither the
		// item nor the recovery move; skip it so the loop always progresses.
		if p.pos == start {
			p.pos++
		}
	}
	p.skipNewlines()
	p.expect(close)
	return tok
}

func startsDecl(k token.Kind) bool {
	switch k {
	case token.KwFn, token.KwType, token.KwImport, token.KwInterface, token.KwConst,
		token.KwLet, token.KwPub, token.KwExtern:
		return true
	}
	return false
}

// separator consumes a comma and/or newlines and reports whether more entries
// may follow. Two commas in a row (an empty entry) are an error.
func (p *parser) separator(close token.Kind) bool {
	hadComma := p.eat(token.Comma)
	hadNewline := p.at(token.Newline)
	p.skipNewlines()
	if p.at(token.Comma) {
		p.errorf("empty list entry")
		p.advance()
		p.skipNewlines()
	}
	if p.at(close) || p.at(token.EOF) {
		return false
	}
	if !hadComma && !hadNewline {
		p.errorf("expected `,` or newline between list entries, found %s", p.describe())
		p.recoverList(close)
		return !p.at(close)
	}
	return true
}

func (p *parser) recoverList(close token.Kind) {
	depth := 0
	for !p.at(token.EOF) {
		switch p.tok().Kind {
		case token.LBrace, token.LParen, token.LBracket:
			depth++
		case token.RBrace, token.RParen, token.RBracket:
			if depth == 0 {
				return
			}
			depth--
		case token.Comma, token.Newline:
			if depth == 0 {
				return
			}
		}
		p.pos++
	}
}

// path parses `a.b.c` and returns a Path node.
func (p *parser) path() NodeID {
	start := uint32(len(p.tree.Extra))
	first := p.pos
	p.tree.Extra = append(p.tree.Extra, p.expect(token.Ident))
	for p.at(token.Dot) && p.peek(1) == token.Ident {
		p.advance()
		p.tree.Extra = append(p.tree.Extra, p.advance())
	}
	return p.tree.add(Node{Kind: Path, Tok: first, Lhs: start, Rhs: uint32(len(p.tree.Extra))})
}

// isUpper reports whether an identifier token starts with an upper-case letter,
// the convention for type names that record-literal detection relies on.
func (p *parser) isUpper(tok uint32) bool {
	text := p.toks[tok].Text(p.tree.File.Src)
	return len(text) > 0 && text[0] >= 'A' && text[0] <= 'Z'
}

// exprList parses call arguments; `...expr` spreads an Array into a variadic
// parameter. `name: expr` names the argument; ordering, duplicates and
// unknown names are a sem concern, not a parse error, since they depend on
// the resolved callee.
func (p *parser) exprList() []NodeID {
	var items []NodeID
	p.layoutList(token.LParen, token.RParen, func() {
		if p.at(token.Ellipsis) {
			dots := p.advance()
			items = append(items, p.node1(Spread, dots, p.expr()))
			return
		}
		if p.at(token.Ident) && p.peek(1) == token.Colon {
			name := p.advance()
			p.advance()
			p.skipNewlines()
			items = append(items, p.node1(NamedArg, name, p.expr()))
			return
		}
		items = append(items, p.expr())
	})
	return items
}
