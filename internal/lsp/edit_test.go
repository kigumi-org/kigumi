package lsp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"kigumi/internal/lsp"
)

func TestRenameAndCodeActions(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "main"), 0o755)
	src := "import text from std/text\n\nfn double(x: Int) -> Int {\n    x * 2\n}\n\nlet total = double(2)\ntotal = double(3)\nlet a = 1\nlet o = a.compareTo(2)\nif o is Less {\n    print \"less\"\n}\nprint \"${total}\"\n"
	path := filepath.Join(root, "main", "main.kg")
	os.WriteFile(path, []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	ctx := protocol.WithClient(context.Background(), &recorder{})
	s := lsp.New(std)
	rootURI := uri.File(root)
	s.Initialize(ctx, initParams(rootURI))
	doc := uri.File(path)
	s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: doc, Text: src}})

	whole := protocol.Range{End: protocol.Position{Line: 20}}
	actions, err := s.CodeAction(ctx, &protocol.CodeActionParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}, Range: whole})
	if err != nil {
		t.Fatal(err)
	}
	titles := map[string]protocol.TextEdit{}
	for _, a := range actions {
		ca := a.(*protocol.CodeAction)
		titles[ca.Title] = ca.Edit.Changes[doc][0]
	}
	if e, ok := titles["Remove unused import"]; !ok || e.Range.Start.Line != 0 || e.Range.End.Line != 1 || e.NewText != "" {
		t.Errorf("unused import fix: %+v (all: %v)", e, keys(titles))
	}
	if e, ok := titles["Declare `mut total`"]; !ok || e.Range.Start.Line != 6 || e.Range.Start.Character != 4 || e.NewText != "mut " {
		t.Errorf("mut fix: %+v (all: %v)", e, keys(titles))
	}
	if e, ok := titles["Write `Ordering.Less`"]; !ok || e.Range.Start.Line != 10 || e.NewText != "Ordering.Less" {
		t.Errorf("variant fix: %+v (all: %v)", e, keys(titles))
	}

	at := protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}, Position: protocol.Position{Line: 6, Character: 14}}
	prep, _ := s.PrepareRename(ctx, &protocol.PrepareRenameParams{TextDocumentPositionParams: at})
	if r, ok := prep.(*protocol.Range); !ok || r.Start.Line != 6 || r.Start.Character != 12 {
		t.Fatalf("prepare rename: %+v", prep)
	}
	edit, err := s.Rename(ctx, &protocol.RenameParams{TextDocumentPositionParams: at, NewName: "twice"})
	if err != nil {
		t.Fatal(err)
	}
	edits := edit.Changes[doc]
	if len(edits) != 3 {
		t.Fatalf("rename edits: %+v", edits)
	}
	lines := map[uint32]bool{}
	for _, e := range edits {
		lines[e.Range.Start.Line] = true
		if e.NewText != "twice" {
			t.Errorf("edit text %q", e.NewText)
		}
	}
	if !lines[2] || !lines[6] || !lines[7] {
		t.Errorf("rename lines: %v", lines)
	}
	if _, err := s.Rename(ctx, &protocol.RenameParams{TextDocumentPositionParams: at, NewName: "not valid"}); err == nil {
		t.Error("invalid identifier accepted")
	}
	stdAt := protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}, Position: protocol.Position{Line: 9, Character: 12}}
	if _, err := s.Rename(ctx, &protocol.RenameParams{TextDocumentPositionParams: stdAt, NewName: "Lower"}); err == nil || !strings.Contains(err.Error(), "module") {
		t.Errorf("renaming a std variant should be refused: %v", err)
	}
}

func keys(m map[string]protocol.TextEdit) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
