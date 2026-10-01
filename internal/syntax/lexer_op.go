package syntax

import "kigumi/internal/token"

// Operator tokens use maximal munch over the character set: the first
// character comes from opStart, later ones from opStart plus '.'. Built-in
// tokens are recognized after the munch; anything else is a user operator.
const opStart = "!~*%+-=<>&^"

var builtinOps = map[string]token.Kind{
	"+": token.Plus, "-": token.Minus, "*": token.Star, "%": token.Percent,
	"&": token.Amp, "^": token.Caret, "!": token.Bang, "~": token.Tilde,
	"=": token.Assign, "==": token.EqEq, "!=": token.NotEq,
	"<": token.Lt, "<=": token.LtEq, ">": token.Gt, ">=": token.GtEq,
	"<<": token.Shl, ">>": token.Shr, "&&": token.AndAnd,
	"->": token.Arrow, "=>": token.FatArrow,
	"+=": token.PlusAssign, "-=": token.MinusAssign, "*=": token.StarAssign,
	"%=": token.PercentAssign, "&=": token.AmpAssign, "^=": token.CaretAssign,
	"<<=": token.ShlAssign, ">>=": token.ShrAssign,
}

func isOperatorStart(c byte) bool {
	for i := range len(opStart) {
		if opStart[i] == c {
			return true
		}
	}
	return false
}

func (lx *lexer) lexOperator() {
	start := lx.pos
	lx.pos++
	for lx.pos < lx.end && (isOperatorStart(lx.src[lx.pos]) || lx.src[lx.pos] == '.') {
		lx.pos++
	}
	text := string(lx.src[start:lx.pos])
	if k, ok := builtinOps[text]; ok {
		lx.emit(k, start, lx.pos)
		return
	}
	if text[0] == '=' {
		lx.errorf(start, lx.pos, "operator cannot start with `=` (§6.2); separate `=` from the following operator")
	}
	lx.emit(token.Operator, start, lx.pos)
}
