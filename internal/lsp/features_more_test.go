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

func openModule(t *testing.T, src string) (*lsp.Server, context.Context, uri.URI, *recorder) {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "main"), 0o755)
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
	return s, ctx, doc, rec
}

func at(doc uri.URI, line, char uint32) protocol.TextDocumentPositionParams {
	return protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}, Position: protocol.Position{Line: line, Character: char}}
}

// Signature help names the callee's parameters and tracks the argument
// under the cursor, also while the call is still unclosed.
func TestSignatureHelp(t *testing.T) {
	src := "/// Scales a point.\nfn scale(x: Int, factor: Int) -> Int {\n    x * factor\n}\n\nlet a = scale(1, 2)\nlet b = scale(3,\n"
	s, ctx, doc, _ := openModule(t, src)
	help, err := s.SignatureHelp(ctx, &protocol.SignatureHelpParams{TextDocumentPositionParams: at(doc, 5, 17)})
	if err != nil || help == nil || len(help.Signatures) != 1 {
		t.Fatalf("signature help: %v %+v", err, help)
	}
	sig := help.Signatures[0]
	if !strings.Contains(sig.Label, "fn scale(x: Int, factor: Int) -> Int") || len(sig.Parameters) != 2 {
		t.Fatalf("signature: %+v", sig)
	}
	if v, ok := help.ActiveParameter.Get(); !ok || v != 1 {
		t.Fatalf("active parameter after the comma = %+v", help.ActiveParameter)
	}
	unclosed, _ := s.SignatureHelp(ctx, &protocol.SignatureHelpParams{TextDocumentPositionParams: at(doc, 6, 16)})
	if unclosed == nil || len(unclosed.Signatures) != 1 {
		t.Fatalf("signature help in an unclosed call: %+v", unclosed)
	}
	if v, _ := unclosed.ActiveParameter.Get(); v != 1 {
		t.Fatalf("active parameter in an unclosed call = %+v", unclosed.ActiveParameter)
	}
}

// A named argument out of declared order still activates
// the parameter it names, not the one at its source position.
func TestSignatureHelpNamedArgs(t *testing.T) {
	src := "fn scale(x: Int, factor: Int) -> Int {\n    x * factor\n}\n\nlet c = scale(factor: 2, x: 1)\n"
	s, ctx, doc, _ := openModule(t, src)
	help, err := s.SignatureHelp(ctx, &protocol.SignatureHelpParams{TextDocumentPositionParams: at(doc, 4, 29)})
	if err != nil || help == nil || len(help.Signatures) != 1 {
		t.Fatalf("signature help: %v %+v", err, help)
	}
	if v, ok := help.ActiveParameter.Get(); !ok || v != 0 {
		t.Fatalf("active parameter inside the reordered `x: 1` = %+v, want 0", help.ActiveParameter)
	}
}

// Hovering a renamed selected import shows the alias at its
// declaration and the source function it stands for at a use site.
func TestHoverImportAlias(t *testing.T) {
	src := "import {decode as decodeUser} from std/json\n\nfn use() -> Unit! {\n    let n: Int = decodeUser(\"1\")?\n}\n"
	s, ctx, doc, _ := openModule(t, src)
	hoverAt := func(pos protocol.TextDocumentPositionParams) string {
		h, err := s.Hover(ctx, &protocol.HoverParams{TextDocumentPositionParams: pos})
		if err != nil {
			t.Fatal(err)
		}
		if h == nil {
			return ""
		}
		return h.Contents.(*protocol.MarkupContent).Value
	}
	if got := hoverAt(at(doc, 0, 20)); !strings.Contains(got, "decodeUser") {
		t.Fatalf("hover on the alias declaration: %q", got)
	}
	if got := hoverAt(at(doc, 3, 20)); !strings.Contains(got, "fn decode") || strings.Contains(got, "decodeUser") {
		t.Fatalf("hover at the alias use site should name the source `decode`, not the alias: %q", got)
	}
}

// Highlights cover the declaration and every use in the file.
func TestDocumentHighlightAndFolding(t *testing.T) {
	src := "fn twice(n: Int) -> Int {\n    n + n\n}\n\nlet v = twice(2)\nprint \"${twice(v)}\"\n"
	s, ctx, doc, _ := openModule(t, src)
	hl, err := s.DocumentHighlight(ctx, &protocol.DocumentHighlightParams{TextDocumentPositionParams: at(doc, 4, 9)})
	if err != nil || len(hl) != 3 {
		t.Fatalf("highlights: %v %+v", err, hl)
	}
	folds, err := s.FoldingRanges(ctx, &protocol.FoldingRangeParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}})
	if err != nil || len(folds) == 0 || folds[0].StartLine != 0 || folds[0].EndLine != 2 {
		t.Fatalf("folding: %v %+v", err, folds)
	}
}

// A near miss gets a quick fix that replaces the name, and the help line
// reaches the editor inside the message.
func TestSuggestionFix(t *testing.T) {
	src := "let count = 3\nprint \"${cont}\"\n"
	s, ctx, doc, rec := openModule(t, src)
	var msg string
	for _, p := range rec.published {
		for _, d := range p.Diagnostics {
			if strings.Contains(string(d.Message.(protocol.String)), "undefined name `cont`") {
				msg = string(d.Message.(protocol.String))
			}
		}
	}
	if !strings.Contains(msg, "help: did you mean `count`?") {
		t.Fatalf("diagnostic lacks the help line: %q", msg)
	}
	actions, err := s.CodeAction(ctx, &protocol.CodeActionParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}, Range: protocol.Range{Start: protocol.Position{Line: 1, Character: 9}, End: protocol.Position{Line: 1, Character: 13}}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range actions {
		if ca, ok := a.(*protocol.CodeAction); ok && ca.Title == "replace with `count`" {
			for _, edits := range ca.Edit.Changes {
				if len(edits) == 1 && edits[0].NewText == "count" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf("no replace action: %+v", actions)
	}
}

// Scope-aware completion offers what is visible with its kind and type,
// and not names from an unrelated inner scope.
func TestScopeCompletion(t *testing.T) {
	src := "fn helper(n: Int) -> Int {\n    n\n}\n\nlet total = 1\nprint \"${tot}\"\n"
	s, ctx, doc, _ := openModule(t, src)
	items := completionLabels(t, s, ctx, doc, 5, 12)
	if it, ok := items["total"]; !ok || it.Kind != protocol.CompletionItemKindVariable {
		t.Fatalf("total missing: %v", labelsOf(items))
	}
	if it, ok := items["helper"]; !ok || it.Kind != protocol.CompletionItemKindFunction {
		t.Fatalf("helper missing or not a function: %v", labelsOf(items))
	}
	if _, ok := items["n"]; ok {
		t.Fatalf("a parameter of another function leaked into completion")
	}
}
