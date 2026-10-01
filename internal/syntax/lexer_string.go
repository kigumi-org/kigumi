package syntax

import (
	"strconv"
	"unicode/utf8"

	"kigumi/internal/token"
)

func (lx *lexer) lexString() {
	start := lx.pos
	_, hinted := lx.hints[start]
	end, ok := lx.scanString(lx.pos)
	lx.pos = min(end, lx.end)
	// A hinted end was already utf8.Valid-checked by the outermost string's
	// initial whole-file lex; rechecking per nesting level would be the
	// O(n²) re-walk the hint avoids.
	if !ok {
		lx.errorf(start, lx.pos, "unterminated string literal")
	} else if !hinted {
		utf8ValidBytes += lx.pos - start
		if !utf8.Valid(lx.src[start:lx.pos]) {
			lx.errorf(start, lx.pos, "string literal contains invalid UTF-8")
		}
	}
	lx.emit(token.String, start, lx.pos)
}

// lx.hints, when set, lets this skip straight to a known end instead of
// rescanning: subExpr lexes a `${...}` body fresh at every nesting level, and
// without the hint a string nested d levels deep would be walked d times.
func (lx *lexer) scanString(pos int) (int, bool) {
	if info, ok := lx.hints[pos]; ok {
		return info.end, info.closed
	}
	pos++
	for pos < lx.end {
		scanOps++
		switch lx.src[pos] {
		case '\\':
			pos = lx.scanEscape(pos)
		case '"':
			return pos + 1, true
		case '$':
			if pos+1 < lx.end && lx.src[pos+1] == '{' {
				var ok bool
				pos, ok = lx.scanInterpolation(pos + 2)
				if !ok {
					return pos, false
				}
				continue
			}
			pos++
		default:
			pos++
		}
	}
	return pos, false
}

func (lx *lexer) scanInterpolation(pos int) (int, bool) {
	depth := 1
	for pos < lx.end {
		switch lx.src[pos] {
		case '{':
			depth++
			pos++
		case '}':
			depth--
			pos++
			if depth == 0 {
				return pos, true
			}
		case '"':
			var ok bool
			if lx.byteStringPrefixAt(pos) {
				pos, ok = lx.scanByteString(pos)
			} else {
				pos, ok = lx.scanString(pos)
			}
			if !ok {
				return pos, false
			}
		default:
			pos++
		}
	}
	lx.errorf(pos, pos, "unterminated interpolation")
	return pos, false
}

// byteStringPrefixAt mirrors the top-level `b"..."` rule in lexer.go's step.
func (lx *lexer) byteStringPrefixAt(pos int) bool {
	if pos < 1 || lx.src[pos-1] != 'b' {
		return false
	}
	return pos < 2 || !isIdentContinueByte(lx.src[pos-2])
}

func (lx *lexer) scanEscape(pos int) int {
	start := pos
	pos++
	if pos >= lx.end {
		return pos
	}
	switch lx.src[pos] {
	case 'n', 't', 'r', '0', '\\', '"', '\'', '$':
		return pos + 1
	case 'u':
		if pos+1 < lx.end && lx.src[pos+1] == '{' {
			digStart := pos + 2
			pos = digStart
			for pos < lx.end && lx.src[pos] != '}' && lx.src[pos] != '"' {
				pos++
			}
			if pos < lx.end && lx.src[pos] == '}' && pos > digStart {
				if v, err := strconv.ParseUint(string(lx.src[digStart:pos]), 16, 32); err == nil && utf8.ValidRune(rune(v)) {
					return pos + 1
				}
			}
		}
	}
	lx.errorf(start, min(pos+1, lx.end), "invalid escape sequence")
	return min(pos+1, lx.end)
}

// lexChar lexes `'x'` and also, since Rust-style lifetimes share the quote
// with char literals, `'a` (a lifetime): an identifier after `'` not itself
// closed by another `'` is a lifetime, matching how rustc disambiguates them.
func (lx *lexer) lexChar() {
	start := lx.pos
	if p := lx.pos + 1; p < lx.end && isIdentStart(lx.src[p]) {
		end := p
		for end < lx.end && isIdentContinueByte(lx.src[end]) {
			end++
		}
		if end >= lx.end || lx.src[end] != '\'' {
			lx.pos = end
			lx.emit(token.Lifetime, start, lx.pos)
			return
		}
	}
	pos := lx.pos + 1
	switch {
	case pos >= lx.end:
	case lx.src[pos] == '\\':
		pos = lx.scanEscape(pos)
	case lx.src[pos] == '\'' || lx.src[pos] == '\n':
	default:
		_, size := utf8.DecodeRune(lx.src[pos:lx.end])
		pos += size
	}
	if pos < lx.end && lx.src[pos] == '\'' {
		lx.pos = pos + 1
		lx.emit(token.Char, start, lx.pos)
		return
	}
	lx.pos = pos
	lx.errorf(start, lx.pos, "invalid character literal")
	lx.emit(token.Char, start, lx.pos)
}
