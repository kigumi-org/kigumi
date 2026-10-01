package lsp

import (
	"context"
	"regexp"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"kigumi/internal/diag"
	"kigumi/internal/errors"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// CodeAction offers the fixes the checker attached to diagnostics that
// overlap the requested range.
func (s *Server) CodeAction(ctx context.Context, p *protocol.CodeActionParams) ([]protocol.CommandOrCodeAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []protocol.CommandOrCodeAction{}
	snap := s.loadDoc(ctx, p.TextDocument.URI)
	if snap.mod == nil {
		return out, nil
	}
	t := snap.fileOf(uriToPath(p.TextDocument.URI))
	if t == nil {
		return out, nil
	}
	var all []diag.Diagnostic
	all = append(all, t.Diags...)
	if snap.res != nil {
		all = append(all, snap.res.Diagnostics(t)...)
	}
	from, to := offset(t.File, p.Range.Start), offset(t.File, p.Range.End)
	kind := protocol.CodeActionKindQuickFix
	for _, d := range all {
		if d.Loc.Span.End < from || d.Loc.Span.Start > to {
			continue
		}
		fixes := d.Fixes
		for _, f := range autoImportFixes(snap, t, d) {
			fixes = append(fixes, f)
		}
		for _, fix := range fixes {
			target := s.fileOf(snap, t.File, fix.Loc)
			edit := protocol.TextEdit{Range: spanRange(target, fix.Loc.Span), NewText: fix.NewText}
			editURI := p.TextDocument.URI
			if target != t.File {
				editURI = uri.File(s.pathOfFile(snap, target.Name))
			}
			out = append(out, &protocol.CodeAction{
				Title:       fix.Title,
				Kind:        &kind,
				Diagnostics: []protocol.Diagnostic{s.toDiagnostic(snap, t.File, d)},
				Edit:        &protocol.WorkspaceEdit{Changes: map[uri.URI][]protocol.TextEdit{editURI: {edit}}},
			})
		}
	}
	return out, nil
}

var identRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// PrepareRename answers with the range of the name under the cursor when it
// belongs to a declaration of this module.
func (s *Server) PrepareRename(ctx context.Context, p *protocol.PrepareRenameParams) (protocol.PrepareRenameResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, t, n, ent := s.entityAt(ctx, p.TextDocumentPositionParams)
	if t == nil || !renamable(snap.res, ent) {
		return nil, nil
	}
	sp := t.Toks[snap.res.Entity(ent).Tok].Span
	if n != 0 {
		sp = nameSpan(t, n)
	}
	r := spanRange(t.File, sp)
	return &r, nil
}

// Rename changes the declaration and every use of an entity, and keeps
// interface requirements and their witnesses in step.
func (s *Server) Rename(ctx context.Context, p *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !identRE.MatchString(p.NewName) {
		return nil, errors.NewErrf("%q is not a valid identifier", p.NewName)
	}
	snap, t, _, ent := s.entityAt(ctx, p.TextDocumentPositionParams)
	if t == nil || ent == 0 {
		return nil, nil
	}
	if !renamable(snap.res, ent) {
		return nil, errors.New("only declarations of this module can be renamed")
	}
	targets := map[sem.EntityID]bool{ent: true}
	if isMethod(snap.res, ent) {
		for _, m := range implementations(snap.res, ent) {
			if renamable(snap.res, m) {
				targets[m] = true
			}
		}
	}
	changes := map[uri.URI][]protocol.TextEdit{}
	add := func(f *token.File, name string, sp token.Span) {
		u := uri.File(s.pathOfFile(snap, name))
		changes[u] = append(changes[u], protocol.TextEdit{Range: spanRange(f, sp), NewText: p.NewName})
	}
	for id := range targets {
		e := snap.res.Entity(id)
		decl := snap.res.Tree(e.File)
		add(decl.File, decl.File.Name, decl.Toks[e.Tok].Span)
	}
	for i := 1; i < len(snap.res.Files); i++ {
		f := &snap.res.Files[i]
		for n, use := range f.Uses {
			if targets[use] {
				add(f.Tree.File, f.Tree.File.Name, nameSpan(f.Tree, syntax.NodeID(n)))
			}
		}
	}
	return &protocol.WorkspaceEdit{Changes: changes}, nil
}

// renamable excludes std, predeclared and unnamed entities.
func renamable(res *sem.Result, ent sem.EntityID) bool {
	if res == nil || ent == 0 {
		return false
	}
	e := res.Entity(ent)
	return e.File != 0 && e.Tok != 0 && e.Flags&sem.EfStd == 0
}
