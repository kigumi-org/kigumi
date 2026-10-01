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

// A note that points into another file reaches the editor as related
// information at that file, not as text folded into the message.
func TestRelatedInformationAcrossFiles(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "cmd", "app"), 0o755)
	os.MkdirAll(filepath.Join(root, "lib"), 0o755)
	os.WriteFile(filepath.Join(root, "cmd", "app", "main.kg"), []byte("import {Shape} from lib\nprint \"${Shape { n: 1 }.n}\"\n"), 0o644)
	os.WriteFile(filepath.Join(root, "lib", "a.kg"), []byte("pub type Shape = {\n    pub n Int\n}\n"), 0o644)
	bPath := filepath.Join(root, "lib", "b.kg")
	bSrc := "pub fn Shape() -> Int {\n    3\n}\n"
	os.WriteFile(bPath, []byte(bSrc), 0o644)
	rec := &recorder{}
	ctx := protocol.WithClient(context.Background(), rec)
	s := lsp.New("../../std")
	if _, err := s.Initialize(ctx, initParams(uri.File(root))); err != nil {
		t.Fatal(err)
	}
	s.Initialized(ctx, &protocol.InitializedParams{})
	s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: uri.File(bPath), Text: bSrc}})
	for _, p := range rec.published {
		if p.URI != uri.File(bPath) {
			continue
		}
		for _, d := range p.Diagnostics {
			if d.Message.(protocol.String) != "`Shape` is already declared in package `lib`" {
				continue
			}
			if len(d.RelatedInformation) != 1 {
				t.Fatalf("related information: %+v", d.RelatedInformation)
			}
			rel := d.RelatedInformation[0]
			if rel.Location.URI != uri.File(filepath.Join(root, "lib", "a.kg")) || rel.Location.Range.Start.Line != 0 || rel.Message != "`Shape` was first declared here" {
				t.Fatalf("related information: %+v", rel)
			}
			return
		}
	}
	t.Fatalf("redeclaration not published for b.kg: %+v", rec.published)
}
