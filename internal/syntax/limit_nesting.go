package syntax

import "kigumi/internal/token"

// maxParseDepth bounds recursive descent so adversarial nesting reports a
// diagnostic instead of overflowing the goroutine stack; it is far above real
// usage and far below the ~500000 levels that overflow Go's default stack.
const maxParseDepth = 10000

// enterDepth guards a recursive-descent entry point; the caller must not
// recurse further when it returns false.
func (p *parser) enterDepth() bool {
	if p.depth >= maxParseDepth {
		p.errorf("nesting is too deep (limit is %d levels)", maxParseDepth)
		return false
	}
	p.depth++
	return true
}

func (p *parser) exitDepth() { p.depth-- }

// matchClosers pairs every LParen/LBracket/LBrace with the index of the token
// that brings the aggregate bracket depth back to what it was before it, in
// one linear pass; matchClosers[i] is 0 when i has no such match. Depth is
// tracked in aggregate across the three bracket kinds, same as the scan this
// replaces, so mismatched-kind input is matched exactly the same way.
func matchClosers(toks []token.Token) []uint32 {
	match := make([]uint32, len(toks))
	var stack []uint32
	for i, t := range toks {
		switch t.Kind {
		case token.LParen, token.LBracket, token.LBrace:
			stack = append(stack, uint32(i))
		case token.RParen, token.RBracket, token.RBrace:
			if n := len(stack); n > 0 {
				match[stack[n-1]] = uint32(i)
				stack = stack[:n-1]
			}
		}
	}
	return match
}
