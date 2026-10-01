package syntax

import (
	"unicode/utf8"

	"kigumi/internal/token"
)

// lexByteString scans a `b"..."` literal: plain ASCII bytes and the
// escapes \xNN \r \n \t \\ \" \0, no interpolation.
func (lx *lexer) lexByteString() {
	start := lx.pos
	lx.pos++
	end, ok := lx.scanByteString(lx.pos)
	lx.pos = min(end, lx.end)
	if !ok {
		lx.errorf(start, lx.pos, "unterminated string literal")
	}
	lx.emit(token.ByteString, start, lx.pos)
}

func (lx *lexer) scanByteString(pos int) (int, bool) {
	pos++
	for pos < lx.end {
		switch {
		case lx.src[pos] == '\\':
			pos = lx.scanByteEscape(pos)
		case lx.src[pos] == '"':
			return pos + 1, true
		case lx.src[pos] < utf8.RuneSelf:
			pos++
		default:
			start := pos
			_, size := utf8.DecodeRune(lx.src[pos:lx.end])
			pos += size
			lx.errorf(start, pos, "non-ASCII character in byte string literal (it would be UTF-8-encoded, not written byte for byte; use \\xNN)")
		}
	}
	return pos, false
}

func (lx *lexer) scanByteEscape(pos int) int {
	start := pos
	pos++
	if pos >= lx.end {
		return pos
	}
	switch lx.src[pos] {
	case 'n', 't', 'r', '0', '\\', '"':
		return pos + 1
	case 'x':
		n := 0
		for n < 2 && pos+1+n < lx.end && isHexDigit(lx.src[pos+1+n]) {
			n++
		}
		if n == 2 {
			return pos + 3
		}
		end := pos + 1 + n
		lx.errorf(start, end, "invalid hex escape (need two hex digits after \\x)")
		return end
	}
	lx.errorf(start, min(pos+1, lx.end), "invalid escape sequence")
	return min(pos+1, lx.end)
}

func isHexDigit(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
