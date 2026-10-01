package lsp

import (
	"context"
	"sort"

	"go.lsp.dev/protocol"

	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// FoldingRanges folds every declaration, block, match, lambda and record
// literal that spans more than one line, from the document's own parse
// so a broken file still folds.
func (s *Server) FoldingRanges(ctx context.Context, p *protocol.FoldingRangeParams) ([]protocol.FoldingRange, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	text, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return nil, nil
	}
	t := syntax.Parse(token.NewFile("", []byte(text)))
	seen := map[[2]int]bool{}
	out := []protocol.FoldingRange{}
	for id := 1; id < len(t.Nodes); id++ {
		n := syntax.NodeID(id)
		switch t.Kind(n) {
		case syntax.FnDecl, syntax.TypeDecl, syntax.Block, syntax.MatchExpr, syntax.Lambda, syntax.RecordLit:
		default:
			continue
		}
		sp := t.Span(n)
		// A node's span stops before its closing brace; fold through it.
		close := sp.End
		for int(close) < len(t.File.Src) && (t.File.Src[close] == ' ' || t.File.Src[close] == '\n' || t.File.Src[close] == '\t') {
			close++
		}
		if int(close) < len(t.File.Src) && t.File.Src[close] == '}' {
			sp.End = close + 1
		}
		start, end := t.File.Line(sp.Start), t.File.Line(max(sp.End-1, sp.Start))
		if end <= start || seen[[2]int{start, end}] {
			continue
		}
		seen[[2]int{start, end}] = true
		out = append(out, protocol.FoldingRange{StartLine: uint32(start - 1), EndLine: uint32(end - 1)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartLine < out[j].StartLine })
	return out, nil
}
