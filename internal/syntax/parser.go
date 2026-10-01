package syntax

import (
	"kigumi/internal/diag"
	"kigumi/internal/token"
)

// Parse always returns a tree. Errors are recorded in Tree.Diags and the
// parser resynchronizes at statement boundaries so later code is still parsed.
func Parse(f *token.File) *Tree {
	var bag diag.Bag
	bag.Limit = 100
	toks := Lex(f.Src, &bag)
	p := &parser{toks: toks, tree: newTree(f, toks), bag: &bag, pos: 1, closer: matchClosers(toks)}
	p.tree.Root = p.parseFile()
	p.tree.interp = p.interp
	p.tree.Diags = bag.Sorted()
	for i := range p.tree.Diags {
		p.tree.Diags[i].Loc.Source = f.ID
	}
	return p.tree
}

type parser struct {
	toks []token.Token
	pos  uint32
	tree *Tree
	bag  *diag.Bag
	// noRecordLit disables `Name { ... }` record literals in condition,
	// iterable and scrutinee positions.
	noRecordLit bool
	lastErr     uint32
	splitAssign bool
	depth       uint32
	// closer[i] is the matching close-bracket index for an open bracket at i,
	// precomputed once so lambda-vs-paren disambiguation is O(1) (see
	// matchClosers).
	closer []uint32
	// interp caches interpolation-range discovery for string literals, keyed
	// by opening-quote offset (see interpolationRanges).
	interp map[int]interpInfo
}

func (p *parser) at(k token.Kind) bool { return p.toks[p.pos].Kind == k }

func (p *parser) tok() token.Token { return p.toks[p.pos] }

func (p *parser) text() string { return p.toks[p.pos].Text(p.tree.File.Src) }

func (p *parser) atIdent(name string) bool {
	return p.at(token.Ident) && p.text() == name
}

// peek returns the kind at offset n from the current token, skipping nothing.
func (p *parser) peek(n int) token.Kind {
	i := int(p.pos) + n
	if i >= len(p.toks) {
		return token.EOF
	}
	return p.toks[i].Kind
}

// peekPastNewlines returns the kind of the next token that is not a newline.
func (p *parser) peekPastNewlines() token.Kind {
	for i := p.pos + 1; int(i) < len(p.toks); i++ {
		if p.toks[i].Kind != token.Newline {
			return p.toks[i].Kind
		}
	}
	return token.EOF
}

func (p *parser) advance() uint32 {
	i := p.pos
	if !p.at(token.EOF) {
		p.pos++
	}
	p.skipComments()
	return i
}

// skipComments skips comments that are not doc comments. Doc comments stay
// visible so declarations can pick them up.
func (p *parser) skipComments() {
	for p.at(token.Comment) || p.at(token.BlockComment) {
		p.pos++
	}
}

func (p *parser) skipNewlines() {
	for p.at(token.Newline) || p.at(token.Comment) || p.at(token.BlockComment) {
		p.pos++
	}
}

func (p *parser) eat(k token.Kind) bool {
	if p.at(k) {
		p.advance()
		return true
	}
	return false
}

func (p *parser) eatIdent(name string) bool {
	if p.atIdent(name) {
		p.advance()
		return true
	}
	return false
}

func (p *parser) expect(k token.Kind) uint32 {
	if k == token.Assign && p.splitAssign {
		p.splitAssign = false
		return p.pos - 1
	}
	if p.at(k) {
		return p.advance()
	}
	p.errorf("expected %s, found %s", k, p.describe())
	return p.pos
}

func (p *parser) describe() string {
	t := p.tok()
	switch t.Kind {
	case token.EOF:
		return "end of file"
	case token.Newline:
		return "end of line"
	case token.Ident, token.Operator, token.Int, token.Float:
		return "`" + t.Text(p.tree.File.Src) + "`"
	}
	return "`" + t.Kind.String() + "`"
}

func (p *parser) errorf(format string, args ...any) {
	p.errorAt(p.tok().Span, format, args...)
}

func (p *parser) errorAt(span token.Span, format string, args ...any) {
	// One error per token keeps cascades quiet.
	if p.pos == p.lastErr && p.bag.Len() > 0 {
		return
	}
	p.lastErr = p.pos
	p.bag.Add(diag.Errorf(diag.Location{Span: span}, sprintf(format, args...)))
}

func (p *parser) warnAt(span token.Span, msg string) {
	p.bag.Add(diag.Warnf(diag.Location{Span: span}, msg))
}

func (p *parser) spanFrom(start uint32) token.Span {
	end := p.pos
	if end > start {
		end--
	}
	return token.Span{Start: p.toks[start].Start, End: p.toks[end].End}
}

// recover skips to the next statement boundary: a newline at brace depth zero,
// an unmatched `}`, or EOF. Parentheses are ignored on purpose so one missing
// `)` cannot swallow the rest of the file. The `}` is left for the enclosing
// block; parseFile consumes it itself.
func (p *parser) recover() {
	depth := 0
	for !p.at(token.EOF) {
		switch p.tok().Kind {
		case token.LBrace:
			depth++
		case token.RBrace:
			if depth == 0 {
				return
			}
			depth--
		case token.Newline:
			if depth == 0 {
				return
			}
		}
		p.pos++
	}
}

// endOfStatement accepts a newline, `;`, `}` (not consumed) or EOF.
func (p *parser) endOfStatement() {
	switch {
	case p.at(token.Newline), p.at(token.Semicolon):
		p.advance()
	case p.at(token.RBrace), p.at(token.EOF):
	default:
		p.errorf("expected end of statement, found %s", p.describe())
		p.recover()
	}
}
