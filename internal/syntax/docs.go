package syntax

import (
	"strings"

	"kigumi/internal/token"
)

// DeclHead is the source of a declaration without its body (functions)
// or with it (types, interfaces, consts), capped at maxHeadLines lines.
func DeclHead(t *Tree, n NodeID) string {
	switch t.Kind(n) {
	case FnDecl, TypeDecl, InterfaceDecl, ConstDecl:
	default:
		return ""
	}
	sp := t.Span(n)
	if body := t.Slot(n, "body"); body != 0 && t.Kind(n) == FnDecl {
		sp.End = t.Span(NodeID(body)).Start
	} else {
		// A node span stops at its last child, so a trailing `]` or `}`
		// can fall outside it; extend to the line that balances an open brace.
		sp.End = t.File.LineSpan(t.File.Line(sp.End - 1)).End
		for depth(t.File.Src[sp.Start:sp.End]) > 0 && int(sp.End) < len(t.File.Src) {
			sp.End = t.File.LineSpan(t.File.Line(sp.End) + 0).End
			if int(sp.End) < len(t.File.Src) && t.File.Src[sp.End] == '\n' {
				sp.End++
			}
		}
	}
	lines := strings.Split(strings.TrimSpace(string(t.File.Src[sp.Start:sp.End])), "\n")
	for len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "///") {
		lines = lines[1:]
	}
	if len(lines) > maxHeadLines {
		lines = append(lines[:maxHeadLines], "    ...")
	}
	return strings.Join(lines, "\n")
}

const maxHeadLines = 40

// DocText joins the `///` lines attached to a declaration; the parser keeps
// only the last one in the doc slot, so the earlier lines are read back
// from the token stream.
func DocText(t *Tree, n NodeID) string {
	last := t.Slot(n, "doc")
	if last == 0 {
		return ""
	}
	first := last
	for i := int(last) - 1; i > 0; i-- {
		switch t.Toks[i].Kind {
		case token.Newline:
			continue
		case token.DocComment:
			first = uint32(i)
			continue
		}
		break
	}
	var lines []string
	for i := first; i <= last; i++ {
		if t.Toks[i].Kind == token.DocComment {
			lines = append(lines, strings.TrimPrefix(strings.TrimPrefix(t.TokText(i), "///"), " "))
		}
	}
	return strings.Join(lines, "\n")
}

func depth(src []byte) int {
	n := 0
	for _, c := range src {
		switch c {
		case '{':
			n++
		case '}':
			n--
		}
	}
	return n
}
