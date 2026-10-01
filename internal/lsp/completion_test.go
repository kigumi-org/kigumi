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

func completionLabels(t *testing.T, s *lsp.Server, ctx context.Context, doc uri.URI, line, char uint32) map[string]protocol.CompletionItem {
	t.Helper()
	items, err := s.Completion(ctx, &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}, Position: protocol.Position{Line: line, Character: char}}})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]protocol.CompletionItem{}
	for _, it := range items.(protocol.CompletionItemSlice) {
		out[it.Label] = it
	}
	return out
}

// After a dot the completion lists the members of the receiver's type and
// nothing else, even while the statement being typed does not parse.
func TestMemberCompletion(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "main"), 0o755)
	src := "type Point = {\n    pub x Int\n    pub y Int\n}\n\nfn Point.norm(self) -> Int {\n    self.x * self.x\n}\n\nlet hoge = 1\nlet p = Point { x: 1, y: 2 }\nhoge.\np.no\nlet toString = 3\n"
	path := filepath.Join(root, "main", "main.kg")
	os.WriteFile(path, []byte(src), 0o644)
	rec := &recorder{}
	ctx := protocol.WithClient(context.Background(), rec)
	s := lsp.New("../../std")
	doc := uri.File(path)
	if _, err := s.Initialize(ctx, initParams(uri.File(root))); err != nil {
		t.Fatal(err)
	}
	s.Initialized(ctx, &protocol.InitializedParams{})
	s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: doc, Text: src}})

	ints := completionLabels(t, s, ctx, doc, 11, 5)
	if _, ok := ints["toFloat"]; !ok {
		t.Fatalf("Int members missing toFloat: %v", labelsOf(ints))
	}
	for _, bad := range []string{"toString", "match", "norm", "hoge", "Point"} {
		if _, ok := ints[bad]; ok {
			t.Fatalf("Int completion offers %q, which Int has no member of: %v", bad, labelsOf(ints))
		}
	}
	points := completionLabels(t, s, ctx, doc, 12, 4)
	if _, ok := points["norm"]; !ok {
		t.Fatalf("Point members missing norm: %v", labelsOf(points))
	}
	if it, ok := points["x"]; !ok || it.Kind != protocol.CompletionItemKindField {
		t.Fatalf("Point members missing field x: %v", labelsOf(points))
	}
	if _, ok := points["toFloat"]; ok {
		t.Fatalf("Point completion offers Int members: %v", labelsOf(points))
	}
	// Hover keeps working around the broken statements.
	hover, err := s.Hover(ctx, &protocol.HoverParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}, Position: protocol.Position{Line: 10, Character: 5}}})
	if err != nil || hover == nil {
		t.Fatalf("hover in a file with parse errors: %v %+v", err, hover)
	}
}

func labelsOf(m map[string]protocol.CompletionItem) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
