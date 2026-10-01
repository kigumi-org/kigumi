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

// Bare completion must not offer a receiver method's name or an
// unimported package's declarations.
func TestBareCompletionFiltersMethodsAndOtherPackages(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "other"), 0o755)
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"app\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "other", "other.kg"), []byte("pub fn PublicHelper() -> Int {\n    1\n}\n\nfn secretHelper() -> Int {\n    2\n}\n"), 0o644)
	src := "type Box = {\n    pub v Int\n}\n\nfn Box.inner(self) -> Int {\n    self.v\n}\n\nfn topLevel() -> Int {\n    1\n}\n\nlet r = 0\n"
	path := filepath.Join(root, "main.kg")
	os.WriteFile(path, []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	ctx := protocol.WithClient(context.Background(), &recorder{})
	s := lsp.New(std)
	s.Initialize(ctx, initParams(uri.File(root)))
	doc := uri.File(path)
	s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: doc, Text: src}})

	items := completionLabels(t, s, ctx, doc, 9, 0)
	if _, ok := items["inner"]; ok {
		t.Errorf("a receiver method's bare name must not be offered: %v", labelsOf(items))
	}
	if _, ok := items["secretHelper"]; ok {
		t.Errorf("a private name from an unimported package must not be offered: %v", labelsOf(items))
	}
	if it, ok := items["topLevel"]; !ok || it.Kind != protocol.CompletionItemKindFunction {
		t.Errorf("topLevel, a plain function of the current package, went missing: %v", labelsOf(items))
	}
	if it, ok := items["PublicHelper"]; !ok || len(it.AdditionalTextEdits) != 1 {
		t.Errorf("PublicHelper should still be offered via auto-import: %+v", it)
	}
}

// A local declaration beats a same-named auto-import suggestion.
func TestBareCompletionLocalNameBeatsImportCandidate(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "other"), 0o755)
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"app\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "other", "other.kg"), []byte("pub fn Widget() -> Int {\n    1\n}\n"), 0o644)
	src := "fn Widget() -> Int {\n    2\n}\n\nfn useIt() -> Int {\n    0\n}\n"
	path := filepath.Join(root, "main.kg")
	os.WriteFile(path, []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	ctx := protocol.WithClient(context.Background(), &recorder{})
	s := lsp.New(std)
	s.Initialize(ctx, initParams(uri.File(root)))
	doc := uri.File(path)
	s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: doc, Text: src}})

	items := completionLabels(t, s, ctx, doc, 5, 0)
	it, ok := items["Widget"]
	if !ok || it.Kind != protocol.CompletionItemKindFunction {
		t.Fatalf("Widget missing or wrong kind: %v", labelsOf(items))
	}
	if len(it.AdditionalTextEdits) != 0 {
		t.Errorf("the local Widget must not be shadowed by an import suggestion: %+v", it)
	}
}

// A name declared twice in the package still resolves to one candidate,
// and a non-pub name from another file of the package is still offered
// (package-private, not file-private).
func TestBareCompletionSemanticPackageScope(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"app\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "helper.kg"), []byte("fn helper() -> Int {\n    1\n}\n\nfn Dup() -> Int {\n    1\n}\n"), 0o644)
	src := "fn Dup() -> Int {\n    2\n}\n\nfn topLevel() -> Int {\n    1\n}\n"
	path := filepath.Join(root, "main.kg")
	os.WriteFile(path, []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	ctx := protocol.WithClient(context.Background(), &recorder{})
	s := lsp.New(std)
	s.Initialize(ctx, initParams(uri.File(root)))
	doc := uri.File(path)
	s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: doc, Text: src}})

	items := completionLabels(t, s, ctx, doc, 5, 0)
	if it, ok := items["helper"]; !ok || it.Kind != protocol.CompletionItemKindFunction {
		t.Errorf("a non-pub name from another file of the same package must still be offered: %v", labelsOf(items))
	}
	if it, ok := items["Dup"]; !ok || it.Kind != protocol.CompletionItemKindFunction {
		t.Errorf("Dup, declared twice in the package, must still resolve to one usable candidate: %v", labelsOf(items))
	}
	if it, ok := items["topLevel"]; !ok || it.Kind != protocol.CompletionItemKindFunction {
		t.Errorf("topLevel went missing: %v", labelsOf(items))
	}
}
