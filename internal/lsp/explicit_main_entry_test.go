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

// An explicit-main cmd/ package is still the process entry and must not be
// offered as an import source for its other pub declarations.
func TestAutoImportSkipsExplicitMainPackage(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "cmd", "tool"), 0o755)
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"app\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "cmd", "tool", "main.kg"), []byte(
		"pub fn helper() -> Int {\n    1\n}\n\nfn main() -> Unit! {\n    print(\"${helper()}\")\n}\n"), 0o644)
	src := "print \"${helper()}\"\n"
	path := filepath.Join(root, "cmd", "other", "main.kg")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	ctx := protocol.WithClient(context.Background(), &recorder{})
	s := lsp.New(std)
	rootURI := uri.File(root)
	s.Initialize(ctx, initParams(rootURI))
	doc := uri.File(path)
	s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: doc, Text: src}})

	actions, err := s.CodeAction(ctx, &protocol.CodeActionParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}, Range: protocol.Range{End: protocol.Position{Line: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range actions {
		ca := a.(*protocol.CodeAction)
		if ca.Title == "import {helper} from app/cmd/tool" {
			t.Fatalf("an explicit-main cmd/ package is an entry file, not a library, and must not be offered as an import source: %q", ca.Title)
		}
	}
}

// With both a script file and an explicit `fn main` in the root package,
// opening the explicit one must resolve it as the entry.
func TestDiagnosticsExplicitMainNoScript(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "tool.kg"), []byte("print \"tool\"\n"), 0o644)
	src := "fn main() -> Unit! {\n    print(\"hi\")\n}\n"
	path := filepath.Join(root, "main.kg")
	os.WriteFile(path, []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	rec := &recorder{}
	ctx := protocol.WithClient(context.Background(), rec)
	s := lsp.New(std)
	doc := uri.File(path)
	s.Initialize(ctx, initParams(uri.File(root)))
	s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: doc, Version: 1, Text: src}})

	for _, p := range rec.published {
		if p.URI != doc {
			continue
		}
		for _, d := range p.Diagnostics {
			t.Errorf("unexpected diagnostic on an explicit-main entry file: %s", d.Message)
		}
	}
}
