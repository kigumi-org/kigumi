package syntax

import (
	"unicode"
	"unicode/utf8"

	"kigumi/internal/token"
)

func (lx *lexer) newline() {
	start := lx.pos
	for lx.pos < lx.end && (lx.src[lx.pos] == '\n' || lx.src[lx.pos] == '\r') {
		lx.pos++
	}
	if n := len(lx.toks); n > 0 && lx.toks[n-1].Kind == token.Newline {
		lx.toks[n-1].End = token.Pos(lx.pos)
		return
	}
	lx.emit(token.Newline, start, lx.pos)
}

func (lx *lexer) lineComment() {
	start := lx.pos
	kind := token.Comment
	if lx.src[start] == '/' && lx.peekByte(2) == '/' && lx.peekByte(3) != '/' {
		kind = token.DocComment
	}
	for lx.pos < lx.end && lx.src[lx.pos] != '\n' {
		lx.pos++
	}
	lx.emit(kind, start, lx.pos)
}

func (lx *lexer) blockComment() {
	start := lx.pos
	depth := 0
	for lx.pos < lx.end {
		switch {
		case lx.src[lx.pos] == '/' && lx.peekByte(1) == '*':
			depth++
			lx.pos += 2
		case lx.src[lx.pos] == '*' && lx.peekByte(1) == '/':
			depth--
			lx.pos += 2
			if depth == 0 {
				lx.emit(token.BlockComment, start, lx.pos)
				return
			}
		default:
			lx.pos++
		}
	}
	lx.errorf(start, lx.pos, "unterminated block comment")
	lx.emit(token.BlockComment, start, lx.pos)
}

func (lx *lexer) lexIdent() {
	start := lx.pos
	for lx.pos < lx.end {
		c := lx.src[lx.pos]
		if isIdentStart(c) || isDigit(c) {
			lx.pos++
			continue
		}
		r, size := utf8.DecodeRune(lx.src[lx.pos:lx.end])
		if r < utf8.RuneSelf || !(unicode.IsLetter(r) || unicode.IsDigit(r)) {
			break
		}
		lx.pos += size
	}
	lx.emit(token.Lookup(string(lx.src[start:lx.pos])), start, lx.pos)
}

func (lx *lexer) lexDots() {
	start := lx.pos
	for lx.pos < lx.end && lx.src[lx.pos] == '.' && lx.pos-start < 3 {
		lx.pos++
	}
	if lx.pos-start == 2 && lx.peekByte(0) == '=' {
		lx.pos++
		lx.emit(token.DotDotEq, start, lx.pos)
		return
	}
	kind := [...]token.Kind{token.Dot, token.DotDot, token.Ellipsis}[lx.pos-start-1]
	lx.emit(kind, start, lx.pos)
}

func (lx *lexer) lexPipe() {
	start := lx.pos
	lx.pos++
	kind := token.Pipe
	switch lx.peekByte(0) {
	case '|':
		kind = token.OrOr
		lx.pos++
	case '>':
		kind = token.PipeGt
		lx.pos++
	case '=':
		kind = token.PipeAssign
		lx.pos++
	}
	lx.emit(kind, start, lx.pos)
}
