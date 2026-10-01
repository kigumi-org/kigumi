package syntax

import "kigumi/internal/token"

func (lx *lexer) lexNumber() {
	start := lx.pos
	kind := token.Int
	if lx.src[lx.pos] == '0' && lx.pos+1 < lx.end {
		switch lx.src[lx.pos+1] {
		case 'x', 'o', 'b':
			lx.pos += 2
			if !lx.digits(lx.src[start+1]) {
				lx.errorf(start, lx.pos, "number needs at least one digit after the base prefix")
			}
			lx.finishNumber(start, kind)
			return
		}
	}
	lx.digits('d')
	if lx.peekByte(0) == '.' && isDigit(lx.peekByte(1)) {
		kind = token.Float
		lx.pos++
		lx.digits('d')
	}
	if c := lx.peekByte(0); c == 'e' || c == 'E' {
		kind = token.Float
		lx.pos++
		if c := lx.peekByte(0); c == '+' || c == '-' {
			lx.pos++
		}
		if !lx.digits('d') {
			lx.errorf(start, lx.pos, "exponent needs digits")
		}
	}
	lx.finishNumber(start, kind)
}

// digits consumes digits of the given base ('d', 'x', 'o', 'b') with `_`
// separators and reports whether at least one digit was seen.
func (lx *lexer) digits(base byte) bool {
	seen := false
	for lx.pos < lx.end {
		c := lx.src[lx.pos]
		if c == '_' {
			lx.pos++
			continue
		}
		if !digitOf(base, c) {
			break
		}
		seen = true
		lx.pos++
	}
	return seen
}

func digitOf(base, c byte) bool {
	switch base {
	case 'b':
		return c == '0' || c == '1'
	case 'o':
		return c >= '0' && c <= '7'
	case 'x':
		return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
	}
	return isDigit(c)
}

func (lx *lexer) finishNumber(start int, kind token.Kind) {
	if lx.pos < lx.end && isIdentStart(lx.src[lx.pos]) {
		bad := lx.pos
		for lx.pos < lx.end && (isIdentStart(lx.src[lx.pos]) || isDigit(lx.src[lx.pos])) {
			lx.pos++
		}
		lx.errorf(bad, lx.pos, "numeric literal has no suffix form; annotate the type instead")
	}
	lx.emit(kind, start, lx.pos)
}
