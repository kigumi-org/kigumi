package syntax

import (
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"
)

// DecodeString decodes the escapes of a string literal's text (the content
// between the quotes, without interpolations). Unknown escapes are kept as is;
// the lexer has already reported them.
func DecodeString(raw string) string {
	if !strings.ContainsRune(raw, '\\') {
		return raw
	}
	var sb strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c != '\\' || i+1 >= len(raw) {
			sb.WriteByte(c)
			continue
		}
		i++
		switch raw[i] {
		case 'n':
			sb.WriteByte('\n')
		case 't':
			sb.WriteByte('\t')
		case 'r':
			sb.WriteByte('\r')
		case '0':
			sb.WriteByte(0)
		case '\\', '"', '\'', '$':
			sb.WriteByte(raw[i])
		case 'u':
			end := strings.IndexByte(raw[i:], '}')
			if i+1 < len(raw) && raw[i+1] == '{' && end > 0 {
				if v, err := strconv.ParseUint(raw[i+2:i+end], 16, 32); err == nil && utf8.ValidRune(rune(v)) {
					sb.WriteRune(rune(v))
					i += end
					continue
				}
			}
			sb.WriteString("\\u")
		default:
			sb.WriteByte('\\')
			sb.WriteByte(raw[i])
		}
	}
	return sb.String()
}

// DecodeByteString decodes a `b"..."` token's full text to its raw
// byte value.
func DecodeByteString(tokText string) []byte {
	return decodeByteEscapes(tokText[2 : len(tokText)-1])
}

// decodeByteEscapes decodes \xNN \r \n \t \\ \" \0; the lexer has already
// reported any other escape or non-ASCII byte.
func decodeByteEscapes(raw string) []byte {
	if !strings.ContainsRune(raw, '\\') {
		return []byte(raw)
	}
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c != '\\' || i+1 >= len(raw) {
			out = append(out, c)
			continue
		}
		i++
		switch raw[i] {
		case 'n':
			out = append(out, '\n')
		case 't':
			out = append(out, '\t')
		case 'r':
			out = append(out, '\r')
		case '0':
			out = append(out, 0)
		case '\\', '"':
			out = append(out, raw[i])
		case 'x':
			if i+2 < len(raw) {
				if v, err := strconv.ParseUint(raw[i+1:i+3], 16, 8); err == nil {
					out = append(out, byte(v))
					i += 2
					continue
				}
			}
			out = append(out, '\\', 'x')
		default:
			out = append(out, '\\', raw[i])
		}
	}
	return out
}

// DecodeChar returns the scalar value of a character literal token text
// such as 'a', '\n' or '\u{1F600}'.
func DecodeChar(tokText string) (rune, bool) {
	if len(tokText) < 3 || tokText[0] != '\'' || tokText[len(tokText)-1] != '\'' {
		return 0, false
	}
	s := DecodeString(tokText[1 : len(tokText)-1])
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError || size != len(s) {
		return 0, false
	}
	return r, true
}

// ParseInt parses an integer literal (with `_` separators and 0x/0o/0b
// prefixes) to arbitrary precision.
func ParseInt(tokText string) (*big.Int, bool) {
	s := strings.ReplaceAll(tokText, "_", "")
	base := 10
	if len(s) > 2 && s[0] == '0' {
		switch s[1] {
		case 'x':
			base, s = 16, s[2:]
		case 'o':
			base, s = 8, s[2:]
		case 'b':
			base, s = 2, s[2:]
		}
	}
	v, ok := new(big.Int).SetString(s, base)
	return v, ok
}

// ParseFloat parses a floating literal to arbitrary precision.
func ParseFloat(tokText string) (*big.Float, bool) {
	v, ok := new(big.Float).SetPrec(256).SetString(strings.ReplaceAll(tokText, "_", ""))
	return v, ok
}
