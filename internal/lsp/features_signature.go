package lsp

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// SignatureHelp shows the callee's signature while the arguments of a
// call are being typed, with the parameter under the cursor active.
func (s *Server) SignatureHelp(ctx context.Context, p *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := s.loadDoc(ctx, p.TextDocument.URI)
	if snap.mod == nil || snap.res == nil {
		return nil, nil
	}
	t := snap.fileOf(uriToPath(p.TextDocument.URI))
	if t == nil {
		return nil, nil
	}
	pos := offset(t.File, p.Position)
	call, active := enclosingCall(t, pos)
	if call == 0 {
		return nil, nil
	}
	info := snap.res.File(t)
	ent := info.Calls[call].Callee
	if ent == 0 {
		ent = info.Uses[syntax.NodeID(t.Nodes[call].Lhs)]
	}
	if ent == 0 || snap.res.Entity(ent).Kind != sem.EntFn {
		return nil, nil
	}
	fn := snap.res.Fn(ent)
	label := describe(snap.res, ent, 0)
	if e := snap.res.Entity(ent); e.File != 0 {
		if head := syntax.DeclHead(snap.res.Tree(e.File), e.Node); head != "" {
			label = head
		}
	}
	sig := protocol.SignatureInformation{Label: label}
	for _, prm := range fn.Params {
		e := snap.res.Entity(prm)
		sig.Parameters = append(sig.Parameters, protocol.ParameterInformation{Label: protocol.String(e.Name + ": " + snap.res.TypeString(e.Type))})
	}
	if doc := syntax.DocText(snap.res.Tree(snap.res.Entity(ent).File), snap.res.Entity(ent).Node); doc != "" && snap.res.Entity(ent).File != 0 {
		sig.Documentation = protocol.String(doc)
	}
	// ArgOrder already places the piped operand (see hir.callWith), so the
	// pipe shifts the index only when sem kept source order.
	if order := info.Calls[call].ArgOrder; order != nil {
		active = activeFromOrder(t, call, active, order)
	} else if info.Calls[call].Piped != 0 {
		active++
	}
	zero := uint32(0)
	param := uint32(min(active, max(len(fn.Params)-1, 0)))
	return &protocol.SignatureHelp{Signatures: []protocol.SignatureInformation{sig}, ActiveSignature: &zero, ActiveParameter: protocol.NewNullable(param)}, nil
}

// enclosingCall finds the innermost call whose argument list holds the
// cursor and counts the arguments before it; a call still missing its
// closing parenthesis is found through its recovered argument list.
func enclosingCall(t *syntax.Tree, pos token.Pos) (syntax.NodeID, int) {
	best, bestLen := syntax.NodeID(0), -1
	active := 0
	for id := 1; id < len(t.Nodes); id++ {
		n := syntax.NodeID(id)
		if t.Kind(n) != syntax.CallExpr {
			continue
		}
		callee := syntax.NodeID(t.Nodes[n].Lhs)
		open := t.Span(callee).End
		if pos <= open {
			continue
		}
		sp := t.Span(n)
		if pos > sp.End && !unclosedAt(t.File.Src, open, pos) {
			continue
		}
		if bestLen >= 0 && sp.Len() >= bestLen {
			continue
		}
		best, bestLen = n, sp.Len()
		active = 0
		for _, a := range t.Children(syntax.NodeID(t.Nodes[n].Rhs)) {
			if t.Span(a).End < pos {
				active++
			}
		}
		if active > 0 && !strings.Contains(string(t.File.Src[open:pos]), ",") {
			active = 0
		}
	}
	return best, active
}

// activeFromOrder maps a source-order argument index to its declared
// parameter position after named arguments reordered the call, falling
// back to sourceIdx when the argument isn't in order yet.
func activeFromOrder(t *syntax.Tree, call syntax.NodeID, sourceIdx int, order []syntax.NodeID) int {
	raw := t.Children(syntax.NodeID(t.Nodes[call].Rhs))
	if sourceIdx < 0 || sourceIdx >= len(raw) {
		return sourceIdx
	}
	arg := raw[sourceIdx]
	if t.Kind(arg) == syntax.NamedArg {
		arg = syntax.NodeID(t.Nodes[arg].Lhs)
	}
	for i, o := range order {
		if o == arg {
			return i
		}
	}
	return sourceIdx
}

// unclosedAt reports whether the parenthesis opened at open is still open
// at pos, so a call being typed counts up to the cursor.
func unclosedAt(src []byte, open, pos token.Pos) bool {
	depth := 0
	for i := open; i < pos && int(i) < len(src); i++ {
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return false
			}
		}
	}
	return depth > 0
}

// DocumentHighlight marks every use of the entity under the cursor in the
// current file, its declaration included.
func (s *Server) DocumentHighlight(ctx context.Context, p *protocol.DocumentHighlightParams) ([]protocol.DocumentHighlight, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, t, _, ent := s.entityAt(ctx, p.TextDocumentPositionParams)
	if t == nil || ent == 0 {
		return nil, nil
	}
	out := []protocol.DocumentHighlight{}
	if e := snap.res.Entity(ent); e.File != 0 && e.Tok != 0 && snap.res.Tree(e.File) == t {
		out = append(out, protocol.DocumentHighlight{Range: spanRange(t.File, t.Toks[e.Tok].Span), Kind: protocol.DocumentHighlightKindText})
	}
	info := snap.res.File(t)
	for n, use := range info.Uses {
		if use == ent {
			out = append(out, protocol.DocumentHighlight{Range: spanRange(t.File, nameSpan(t, syntax.NodeID(n))), Kind: protocol.DocumentHighlightKindText})
		}
	}
	return out, nil
}
