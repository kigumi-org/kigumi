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

func contains(outer, inner protocol.Range) bool {
	before := inner.Start.Line < outer.Start.Line || inner.Start.Line == outer.Start.Line && inner.Start.Character < outer.Start.Character
	after := inner.End.Line > outer.End.Line || inner.End.Line == outer.End.Line && inner.End.Character > outer.End.Character
	return !before && !after
}

// TestSymbolRanges checks, over every shipped .kg file, that each
// symbol's selection range lies inside its range (VS Code rejects the
// whole response otherwise).
func TestSymbolRanges(t *testing.T) {
	ctx := protocol.WithClient(context.Background(), &recorder{})
	s := lsp.New("")
	for _, dir := range []string{"../../std", "../../testdata"} {
		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(path) != ".kg" {
				return nil
			}
			src, _ := os.ReadFile(path)
			u := uri.File(path)
			s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: u, Text: string(src)}})
			res, _ := s.DocumentSymbol(ctx, &protocol.DocumentSymbolParams{TextDocument: protocol.TextDocumentIdentifier{URI: u}})
			for _, sym := range res.(protocol.DocumentSymbolSlice) {
				if !contains(sym.Range, sym.SelectionRange) {
					t.Errorf("%s: %s range %v sel %v", path, sym.Name, sym.Range, sym.SelectionRange)
				}
				for _, c := range sym.Children {
					if !contains(c.Range, c.SelectionRange) {
						t.Errorf("%s: %s.%s range %v sel %v", path, sym.Name, c.Name, c.Range, c.SelectionRange)
					}
				}
			}
			return nil
		})
	}
}
