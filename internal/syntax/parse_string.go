package syntax

import "kigumi/internal/token"

// stringLit parses a string token and its `${...}` interpolations. Each
// interpolation is lexed and parsed in place; its tokens are appended to the
// token array so node indices stay valid.
func (p *parser) stringLit() NodeID {
	tok := p.expect(token.String)
	t := p.toks[tok]
	src := p.tree.File.Src
	if p.interp == nil {
		p.interp = make(map[int]interpInfo)
	}
	var parts []NodeID
	for _, r := range interpolationRanges(src, int(t.Start), int(t.End), p.interp) {
		parts = append(parts, p.subExpr(r[0], r[1]))
	}
	var list NodeID
	if len(parts) > 0 {
		list = p.tree.addList(List, tok, parts)
	}
	return p.node1(StringLit, tok, list)
}

// byteStringLit parses a `b"..."` token; unlike stringLit it never
// interpolates.
func (p *parser) byteStringLit() NodeID {
	return p.leaf(ByteStringLit, p.expect(token.ByteString))
}

type interpInfo struct {
	ranges [][2]int
	end    int
	closed bool
}

// scanOps counts bytes visited across interpolationRanges, matchBrace and
// skipString; interp_perf_test.go asserts deep nesting scans linearly,
// without timing wall-clock.
var scanOps int

// utf8ValidBytes counts bytes passed to utf8.Valid by lexString;
// interp_perf_test.go asserts it grows with source size, not size*depth,
// since a hint hit skips the recheck (see lexString).
var utf8ValidBytes int

// interpolationRanges returns [start,end) ranges of expressions inside
// `${...}` within one string literal src[start:end]. When out is non-nil it
// caches by string start and records nested strings' ranges too, avoiding
// the per-level rescan that made deep nesting O(n²).
func interpolationRanges(src []byte, start, end int, out map[int]interpInfo) [][2]int {
	if out != nil {
		if info, ok := out[start]; ok {
			return info.ranges
		}
	}
	var ranges [][2]int
	i := start + 1
	for i < end-1 {
		scanOps++
		switch src[i] {
		case '\\':
			i += 2
		case '$':
			if i+1 < end && src[i+1] == '{' {
				close := matchBrace(src, i+2, end, out)
				ranges = append(ranges, [2]int{i + 2, close})
				i = close + 1
				continue
			}
			i++
		default:
			i++
		}
	}
	if out != nil {
		out[start] = interpInfo{ranges: ranges, end: end, closed: true}
	}
	return ranges
}

func matchBrace(src []byte, pos, end int, out map[int]interpInfo) int {
	depth := 1
	for pos < end {
		scanOps++
		switch src[pos] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return pos
			}
		case '"':
			pos = skipString(src, pos, end, out)
			continue
		}
		pos++
	}
	return end
}

func skipString(src []byte, pos, end int, out map[int]interpInfo) int {
	start := pos
	pos++
	var ranges [][2]int
	for pos < end {
		scanOps++
		switch src[pos] {
		case '\\':
			pos += 2
			continue
		case '"':
			pos++
			if out != nil {
				out[start] = interpInfo{ranges: ranges, end: pos, closed: true}
			}
			return pos
		case '$':
			if pos+1 < end && src[pos+1] == '{' {
				close := matchBrace(src, pos+2, end, out)
				ranges = append(ranges, [2]int{pos + 2, close})
				pos = close + 1
				continue
			}
		}
		pos++
	}
	if out != nil {
		out[start] = interpInfo{ranges: ranges, end: pos, closed: false}
	}
	return pos
}

// subExpr lexes src[start:end] as a standalone expression and parses it with
// the same parser state. The sub-token stream ends with its own EOF.
func (p *parser) subExpr(start, end int) NodeID {
	if start >= end {
		p.errorAt(token.Span{Start: token.Pos(start), End: token.Pos(end)}, "empty interpolation")
		return 0
	}
	base := uint32(len(p.toks))
	sub := lexRange(p.tree.File.Src, start, end, p.bag, p.interp)
	p.toks = append(p.toks, sub...)
	p.tree.Toks = p.toks
	subCloser := matchClosers(sub)
	for i, m := range subCloser {
		if m != 0 {
			subCloser[i] = m + base
		}
	}
	p.closer = append(p.closer, subCloser...)
	savedPos, savedNR := p.pos, p.noRecordLit
	p.pos, p.noRecordLit = base, false
	p.skipNewlines()
	e := p.expr()
	p.skipNewlines()
	if !p.at(token.EOF) {
		p.errorf("unexpected %s in interpolation", p.describe())
	}
	p.pos, p.noRecordLit = savedPos, savedNR
	return e
}
