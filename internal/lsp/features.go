package lsp

import (
	"context"

	"go.lsp.dev/protocol"

	"kigumi/internal/driver"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// nodeAt finds the innermost name-bearing node covering a byte offset.
func nodeAt(t *syntax.Tree, pos token.Pos) syntax.NodeID {
	best, bestLen := syntax.NodeID(0), -1
	for id := 1; id < len(t.Nodes); id++ {
		n := syntax.NodeID(id)
		switch t.Kind(n) {
		case syntax.Ident, syntax.MemberExpr, syntax.PatBind, syntax.Param, syntax.PatCtor, syntax.TypePath, syntax.FieldInit, syntax.PatField:
		default:
			continue
		}
		sp := t.Span(n)
		if t.Kind(n) == syntax.MemberExpr {
			sp = t.Toks[t.Nodes[n].Tok].Span
		}
		if pos >= sp.Start && pos <= sp.End && (bestLen < 0 || sp.Len() < bestLen) {
			best, bestLen = n, sp.Len()
		}
	}
	return best
}

// entityAt resolves the entity a position refers to or declares.
func (s *Server) entityAt(ctx context.Context, p protocol.TextDocumentPositionParams) (*snapshot, *syntax.Tree, syntax.NodeID, sem.EntityID) {
	snap := s.loadDoc(ctx, p.TextDocument.URI)
	if snap.mod == nil || snap.res == nil {
		return snap, nil, 0, 0
	}
	t := snap.fileOf(uriToPath(p.TextDocument.URI))
	if t == nil {
		return snap, nil, 0, 0
	}
	pos := offset(t.File, p.Position)
	n := nodeAt(t, pos)
	if n == 0 {
		return snap, t, 0, declaredAt(snap.res, t, pos)
	}
	info := snap.res.File(t)
	ent := info.Uses[n]
	if ent == 0 {
		ent = info.Defs[n]
	}
	return snap, t, n, ent
}

// declaredAt finds the entity whose declaring name token covers pos;
// declaration names are tokens, not nodes, so nodeAt cannot see them.
func declaredAt(res *sem.Result, t *syntax.Tree, pos token.Pos) sem.EntityID {
	for i := range res.Entities {
		e := &res.Entities[i]
		if e.File == 0 || e.Tok == 0 || res.Tree(e.File) != t {
			continue
		}
		if sp := t.Toks[e.Tok].Span; pos >= sp.Start && pos <= sp.End {
			return sem.EntityID(i)
		}
	}
	return 0
}

func (s *Server) Hover(ctx context.Context, p *protocol.HoverParams) (*protocol.Hover, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, t, n, ent := s.entityAt(ctx, p.TextDocumentPositionParams)
	if t == nil || n == 0 && ent == 0 {
		return nil, nil
	}
	info := snap.res.File(t)
	text := ""
	switch {
	case ent != 0:
		text = hoverText(snap.res, ent, info.Types[n])
	case info.Types[n] != 0:
		text = "```kigumi\n" + snap.res.TypeString(info.Types[n]) + "\n```"
	}
	if text == "" {
		return nil, nil
	}
	sp := t.Span(n)
	if n == 0 {
		sp = t.Toks[snap.res.Entity(ent).Tok].Span
	}
	r := spanRange(t.File, sp)
	return &protocol.Hover{Contents: &protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: text}, Range: &r}, nil
}

func (s *Server) Definition(ctx context.Context, p *protocol.DefinitionParams) (protocol.DefinitionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, t, _, ent := s.entityAt(ctx, p.TextDocumentPositionParams)
	if t == nil || ent == 0 {
		return nil, nil
	}
	if loc := s.entityLocation(snap, ent); loc != nil {
		return loc, nil
	}
	return nil, nil
}

// RangeFormatting formats the whole document: the canonical form is
// defined per file, so a partial reformat would leave inconsistent output.
func (s *Server) RangeFormatting(ctx context.Context, p *protocol.DocumentRangeFormattingParams) ([]protocol.TextEdit, error) {
	return s.Formatting(ctx, &protocol.DocumentFormattingParams{TextDocument: p.TextDocument, Options: p.Options})
}

func (s *Server) Formatting(ctx context.Context, p *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	text, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return nil, nil
	}
	out, err := driver.Format(driver.Source{Path: uriToPath(p.TextDocument.URI), Src: []byte(text)})
	if err != nil || out == text {
		return []protocol.TextEdit{}, nil
	}
	return []protocol.TextEdit{{Range: protocol.Range{End: position(fileOfText(text), token.Pos(len(text)))}, NewText: out}}, nil
}
