package lsp_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"kigumi/internal/lsp"
)

// Diagnostics published after two rapid edits carry the latest version,
// not a stale one from an earlier edit.
func TestPublishDiagnosticsVersion(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "main"), 0o755)
	src := "fn double(x: Int) -> Int {\n    x * 2\n}\n"
	path := filepath.Join(root, "main", "main.kg")
	os.WriteFile(path, []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	rec := &recorder{}
	ctx := protocol.WithClient(context.Background(), rec)
	s := lsp.New(std)
	doc := uri.File(path)

	if _, err := s.Initialize(ctx, initParams(uri.File(root))); err != nil {
		t.Fatal(err)
	}
	s.Initialized(ctx, &protocol.InitializedParams{})
	s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: doc, Version: 1, Text: src}})

	change := func(version int32, text string) {
		s.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: doc},
				Version:                version,
			},
			ContentChanges: []protocol.TextDocumentContentChangeEvent{&protocol.TextDocumentContentChangeWholeDocument{Text: text}},
		})
	}
	change(2, "fn double(x: Int) -> Int {\n    x * 3\n}\n")
	change(3, "fn double(x: Int) -> Int {\n    x * 4\n}\n")

	var last *protocol.PublishDiagnosticsParams
	for _, p := range rec.published {
		if p.URI == doc {
			last = p
		}
	}
	if last == nil {
		t.Fatal("no diagnostics published for the changed document")
	}
	if v, ok := last.Version.Get(); !ok || v != 3 {
		t.Fatalf("published version = %v, ok=%v, want 3", v, ok)
	}
}
