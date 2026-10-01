package syntax

import (
	"unicode"
	"unicode/utf8"

	"kigumi/internal/diag"
	"kigumi/internal/token"
)

type lexer struct {
	src   []byte
	pos   int
	end   int
	toks  []token.Token
	diags *diag.Bag
	space bool
	// hints carries known string-literal ends, keyed by opening-quote offset,
	// discovered up front by the parser's interpolation scan (see
	// scanInterp in parse_string.go). nil for the initial file-wide lex.
	hints map[int]interpInfo
}

// Lex tokenizes the whole source. Comments and newlines are kept as tokens so
// the tree stays lossless; runs of newlines collapse into one token.
func Lex(src []byte, bag *diag.Bag) []token.Token {
	toks := []token.Token{{Kind: token.BOF}}
	return append(toks, lexRange(src, 0, len(src), bag, nil)...)
}

func lexRange(src []byte, start, end int, bag *diag.Bag, hints map[int]interpInfo) []token.Token {
	lx := &lexer{src: src, pos: start, end: end, diags: bag, hints: hints}
	for lx.pos < lx.end {
		lx.step()
	}
	lx.emit(token.EOF, lx.end, lx.end)
	return lx.toks
}

func (lx *lexer) emit(kind token.Kind, start, end int) {
	var flags uint8
	if lx.space {
		flags |= token.SpaceBefore
	}
	lx.space = false
	lx.toks = append(lx.toks, token.Token{
		Kind: kind, Flags: flags,
		Span: token.Span{Start: token.Pos(start), End: token.Pos(end)},
	})
}

func (lx *lexer) errorf(start, end int, msg string) {
	lx.diags.Add(diag.Errorf(diag.Location{Span: token.Span{Start: token.Pos(start), End: token.Pos(end)}}, msg))
}

func (lx *lexer) peekByte(off int) byte {
	if lx.pos+off < lx.end {
		return lx.src[lx.pos+off]
	}
	return 0
}

func (lx *lexer) step() {
	c := lx.src[lx.pos]
	start := lx.pos
	switch {
	case c == ' ' || c == '\t' || c == '\r':
		lx.pos++
		lx.space = true
	case c == '\n':
		lx.newline()
	case c == '/' && lx.peekByte(1) == '/':
		lx.lineComment()
	case c == '/' && lx.peekByte(1) == '*':
		lx.blockComment()
	case c == '#':
		lx.lineComment()
	case c == '"':
		lx.lexString()
	case c == 'b' && lx.peekByte(1) == '"':
		lx.lexByteString()
	case c == '\'':
		lx.lexChar()
	case isDigit(c):
		lx.lexNumber()
	case isIdentStart(c):
		lx.lexIdent()
	case c == '.':
		lx.lexDots()
	case c == '|':
		lx.lexPipe()
	case c == '?':
		if lx.peekByte(1) == '.' {
			lx.pos += 2
			lx.emit(token.QuestionDot, start, lx.pos)
		} else {
			lx.pos++
			lx.emit(token.Question, start, lx.pos)
		}
	case c == '/':
		if lx.peekByte(1) == '=' {
			lx.pos += 2
			lx.emit(token.SlashAssign, start, lx.pos)
		} else {
			lx.pos++
			lx.emit(token.Slash, start, lx.pos)
		}
	case isOperatorStart(c):
		lx.lexOperator()
	default:
		if k, ok := singleCharKinds[c]; ok {
			lx.pos++
			lx.emit(k, start, lx.pos)
			return
		}
		r, size := utf8.DecodeRune(lx.src[lx.pos:lx.end])
		if unicode.IsLetter(r) {
			lx.lexIdent()
			return
		}
		lx.pos += size
		lx.errorf(start, lx.pos, "unexpected character")
		lx.emit(token.Invalid, start, lx.pos)
	}
}

var singleCharKinds = map[byte]token.Kind{
	'(': token.LParen, ')': token.RParen, '[': token.LBracket, ']': token.RBracket,
	'{': token.LBrace, '}': token.RBrace, ',': token.Comma, ':': token.Colon,
	';': token.Semicolon, '@': token.At, '$': token.Dollar,
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentContinueByte(c byte) bool {
	return isIdentStart(c) || isDigit(c) || c >= utf8.RuneSelf
}
