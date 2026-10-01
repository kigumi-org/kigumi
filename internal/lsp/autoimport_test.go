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

// Offers imports for an unresolved qualifier and name as quick fixes, and
// as completion items that carry the import edit.
func TestAutoImport(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "util"), 0o755)
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"app\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "util", "util.kg"), []byte("pub fn helper() -> Int {\n    1\n}\n\nfn hidden() -> Int {\n    2\n}\n"), 0o644)
	src := "#!/usr/bin/env kigumi\n\nimport {sqrt} from std/math\n\nlet s = text.trim(\" a \")\nprint \"${helper()} ${s}\"\n"
	path := filepath.Join(root, "main.kg")
	os.WriteFile(path, []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	ctx := protocol.WithClient(context.Background(), &recorder{})
	s := lsp.New(std)
	rootURI := uri.File(root)
	s.Initialize(ctx, initParams(rootURI))
	doc := uri.File(path)
	s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: doc, Text: src}})

	actions, err := s.CodeAction(ctx, &protocol.CodeActionParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}, Range: protocol.Range{End: protocol.Position{Line: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	titles := map[string]protocol.TextEdit{}
	for _, a := range actions {
		ca := a.(*protocol.CodeAction)
		titles[ca.Title] = ca.Edit.Changes[doc][0]
	}
	if e, ok := titles["import text from std/text"]; !ok || e.Range.Start.Line != 3 || e.NewText != "import text from std/text\n" {
		t.Errorf("qualifier fix: %+v (all: %v)", e, keys(titles))
	}
	if e, ok := titles["import {helper} from app/util"]; !ok || e.NewText != "import {helper} from app/util\n" {
		t.Errorf("name fix: %+v (all: %v)", e, keys(titles))
	}
	if _, ok := titles["import {hidden} from app/util"]; ok {
		t.Error("private names must not be offered")
	}

	items, err := s.Completion(ctx, &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}, Position: protocol.Position{Line: 5, Character: 0}}})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]protocol.CompletionItem{}
	for _, it := range items.(protocol.CompletionItemSlice) {
		found[it.Label] = it
	}
	if it, ok := found["json"]; !ok || len(it.AdditionalTextEdits) != 1 || !strings.HasPrefix(it.AdditionalTextEdits[0].NewText, "import json from std/json") {
		t.Errorf("json completion should add its import: %+v", it)
	}
	if it, ok := found["floor"]; !ok || len(it.AdditionalTextEdits) != 1 || it.AdditionalTextEdits[0].NewText != "{floor, sqrt}" {
		t.Errorf("floor should join the existing std/math import: %+v", it)
	}
	if it := found["sqrt"]; len(it.AdditionalTextEdits) != 0 {
		t.Errorf("an imported name needs no edit: %+v", it)
	}
}
