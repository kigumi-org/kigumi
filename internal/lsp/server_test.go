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

// recorder is the client side: it keeps every published diagnostic and
// log line.
type recorder struct {
	protocol.UnimplementedClient
	published []*protocol.PublishDiagnosticsParams
	logs      []*protocol.LogMessageParams
}

func (r *recorder) LogMessage(_ context.Context, p *protocol.LogMessageParams) error {
	r.logs = append(r.logs, p)
	return nil
}

func (r *recorder) PublishDiagnostics(_ context.Context, p *protocol.PublishDiagnosticsParams) error {
	r.published = append(r.published, p)
	return nil
}

func TestServer(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "main"), 0o755)
	src := "fn double(x: Int) -> Int {\n    x * 2\n}\n\nlet value = double(21)\nlet bad: String = value\nprint \"v=${value}\"\nlet items = Array.of(value)\n"
	path := filepath.Join(root, "main", "main.kg")
	os.WriteFile(path, []byte(src), 0o644)
	os.MkdirAll(filepath.Join(root, "nested", "cmd", "app"), 0o755)
	os.MkdirAll(filepath.Join(root, "nested", "lib"), 0o755)
	os.WriteFile(filepath.Join(root, "nested", "cmd", "app", "main.kg"), []byte("import {answer} from lib\nprint \"${answer()}\"\n"), 0o644)
	os.WriteFile(filepath.Join(root, "nested", "lib", "lib.kg"), []byte("pub fn answer() -> Int {\n    42\n}\n"), 0o644)
	std, _ := filepath.Abs("../../std")
	rec := &recorder{}
	ctx := protocol.WithClient(context.Background(), rec)
	s := lsp.New("../../std")
	doc := uri.File(path)
	rootURI := uri.File(root)

	if _, err := s.Initialize(ctx, initParams(rootURI)); err != nil {
		t.Fatal(err)
	}
	if len(rec.logs) != 1 || rec.logs[0].Type != protocol.MessageTypeInfo || !strings.Contains(rec.logs[0].Message, std) {
		t.Fatalf("initialize log: %+v", rec.logs)
	}
	s.Initialized(ctx, &protocol.InitializedParams{})
	if len(rec.published) != 1 || rec.published[0].URI != doc || len(rec.published[0].Diagnostics) != 1 {
		t.Fatalf("workspace diagnostics before opening a file (nested module must be clean): %+v", rec.published)
	}
	s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: doc, Text: src}})
	found := false
	for _, p := range rec.published {
		for _, d := range p.Diagnostics {
			if strings.Contains(string(d.Message.(protocol.String)), "expected `String`, found `Int`") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("type error not reported: %+v", rec.published)
	}

	at := protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}, Position: protocol.Position{Line: 4, Character: 13}}
	hover, err := s.Hover(ctx, &protocol.HoverParams{TextDocumentPositionParams: at})
	if err != nil || hover == nil || !strings.Contains(hover.Contents.(*protocol.MarkupContent).Value, "fn double(x: Int) -> Int") {
		t.Fatalf("hover: %v %+v", err, hover)
	}
	def, _ := s.Definition(ctx, &protocol.DefinitionParams{TextDocumentPositionParams: at})
	if loc, ok := def.(*protocol.Location); !ok || loc.URI != doc || loc.Range.Start.Line != 0 {
		t.Fatalf("definition: %+v", def)
	}
	inStd := protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}, Position: protocol.Position{Line: 7, Character: 18}}
	stdDef, _ := s.Definition(ctx, &protocol.DefinitionParams{TextDocumentPositionParams: inStd})
	if loc, ok := stdDef.(*protocol.Location); !ok || loc.URI != uri.File(filepath.Join(std, "array", "array.kg")) {
		t.Fatalf("definition of a std symbol must point at the std file on disk: %+v", stdDef)
	}
	syms, _ := s.DocumentSymbol(ctx, &protocol.DocumentSymbolParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}})
	if list := syms.(protocol.DocumentSymbolSlice); len(list) != 1 || list[0].Name != "double" {
		t.Fatalf("symbols: %+v", syms)
	}
	items, _ := s.Completion(ctx, &protocol.CompletionParams{TextDocumentPositionParams: at})
	labels := map[string]bool{}
	for _, it := range items.(protocol.CompletionItemSlice) {
		labels[it.Label] = true
	}
	if !labels["double"] || !labels["match"] {
		t.Fatalf("completion lacks names: %v", labels)
	}

	refs, _ := s.References(ctx, &protocol.ReferenceParams{TextDocumentPositionParams: at, Context: protocol.ReferenceContext{IncludeDeclaration: true}})
	if len(refs) != 2 || refs[0].Range.Start.Line != 0 || refs[1].Range.Start.Line != 4 {
		t.Fatalf("references: %+v", refs)
	}
	syms2, _ := s.Symbols(ctx, &protocol.WorkspaceSymbolParams{Query: "dbl"})
	if list := syms2.(protocol.SymbolInformationSlice); len(list) != 1 || list[0].Name != "double" || *list[0].ContainerName != "main" {
		t.Fatalf("workspace symbols: %+v", syms2)
	}

	messy := "fn   double(x: Int)->Int {\n  x*2\n}\n"
	s.DidChange(ctx, &protocol.DidChangeTextDocumentParams{TextDocument: protocol.VersionedTextDocumentIdentifier{TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: doc}}, ContentChanges: []protocol.TextDocumentContentChangeEvent{&protocol.TextDocumentContentChangeWholeDocument{Text: messy}}})
	edits, _ := s.Formatting(ctx, &protocol.DocumentFormattingParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}})
	if len(edits) != 1 || !strings.HasPrefix(edits[0].NewText, "fn double(x: Int) -> Int {") {
		t.Fatalf("formatting: %+v", edits)
	}
}

func TestImplementation(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "shapes"), 0o755)
	src := "pub interface Shape {\n    fn area(self) -> Int\n}\n\npub type Square = {\n    pub side Int\n}\n\nfn Square.area(self) -> Int {\n    self.side * self.side\n}\n\npub type Dot = {\n    pub x Int\n}\n"
	path := filepath.Join(root, "shapes", "shapes.kg")
	os.WriteFile(path, []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	ctx := protocol.WithClient(context.Background(), &recorder{})
	s := lsp.New(std)
	rootURI := uri.File(root)
	s.Initialize(ctx, initParams(rootURI))
	doc := uri.File(path)
	s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: doc, Text: src}})
	lines := func(pos protocol.Position) []uint32 {
		res, _ := s.Implementation(ctx, &protocol.ImplementationParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}, Position: pos}})
		var out []uint32
		for _, loc := range res.(protocol.LocationSlice) {
			out = append(out, loc.Range.Start.Line)
		}
		return out
	}
	if got := lines(protocol.Position{Line: 0, Character: 15}); len(got) != 1 || got[0] != 4 {
		t.Fatalf("implementations of Shape: %v", got)
	}
	if got := lines(protocol.Position{Line: 1, Character: 8}); len(got) != 1 || got[0] != 8 {
		t.Fatalf("implementations of Shape.area: %v", got)
	}
	if got := lines(protocol.Position{Line: 4, Character: 10}); len(got) != 1 || got[0] != 0 {
		t.Fatalf("interfaces of Square: %v", got)
	}
	if got := lines(protocol.Position{Line: 8, Character: 11}); len(got) != 1 || got[0] != 1 {
		t.Fatalf("requirements of Square.area: %v", got)
	}
	if got := lines(protocol.Position{Line: 12, Character: 10}); len(got) != 0 {
		t.Fatalf("Dot conforms to nothing: %v", got)
	}
	syms, _ := s.Symbols(ctx, &protocol.WorkspaceSymbolParams{Query: "area"})
	if list := syms.(protocol.SymbolInformationSlice); len(list) != 2 || list[0].Name != "Shape.area" && list[1].Name != "Shape.area" {
		t.Fatalf("workspace symbols for area: %+v", syms)
	}
}

func TestHoverDocs(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "main"), 0o755)
	src := "import array from std/array\n\n/// Doubles x.\n/// Twice.\nfn double(x: Int) -> Int {\n    x * 2\n}\n\nlet xs = Array.of(1, 2)\nlet n = double(xs.len())\nprint \"${n}\"\n"
	path := filepath.Join(root, "main", "main.kg")
	os.WriteFile(path, []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	ctx := protocol.WithClient(context.Background(), &recorder{})
	s := lsp.New(std)
	rootURI := uri.File(root)
	s.Initialize(ctx, initParams(rootURI))
	doc := uri.File(path)
	s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: doc, Text: src}})
	hoverAt := func(line, col uint32) string {
		h, _ := s.Hover(ctx, &protocol.HoverParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}, Position: protocol.Position{Line: line, Character: col}}})
		if h == nil {
			return ""
		}
		return h.Contents.(*protocol.MarkupContent).Value
	}
	if got := hoverAt(9, 9); got != "```kigumi\nfn double(x: Int) -> Int\n```\n\nDoubles x.\nTwice." {
		t.Fatalf("user fn hover: %q", got)
	}
	if got := hoverAt(8, 15); !strings.Contains(got, "pub pure fn Array.of[T](items: ...T) -> Array[T]\n```\n\nCreates an array holding the given items in order.") {
		t.Fatalf("std fn hover: %q", got)
	}
	if got := hoverAt(9, 19); !strings.Contains(got, "pub pure noalloc fn Array[T].len(self) -> usize") || !strings.Contains(got, "Number of elements.") {
		t.Fatalf("std method hover: %q", got)
	}
	if got := hoverAt(10, 9); got != "```kigumi\nlocal n: Int\n```" {
		t.Fatalf("variable hover: %q", got)
	}
}

// TestHoverConstParam guards against hovering a const generic parameter's
// use in a function body showing the enclosing function's own signature
// instead: the hidden local it resolves to must carry its own
// declaration node, not the function's.
func TestHoverConstParam(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "main"), 0o755)
	src := "fn len[T, const N: usize](a: &FixedArray[T, N]) -> usize {\n    N\n}\n\nfn main() -> Unit! {\n    let a = FixedArray.filled[i32, 3](0)\n    print \"${len(&a)}\"\n}\n"
	path := filepath.Join(root, "main", "main.kg")
	os.WriteFile(path, []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	ctx := protocol.WithClient(context.Background(), &recorder{})
	s := lsp.New(std)
	s.Initialize(ctx, initParams(uri.File(root)))
	doc := uri.File(path)
	s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{TextDocument: protocol.TextDocumentItem{URI: doc, Text: src}})
	h, err := s.Hover(ctx, &protocol.HoverParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: doc}, Position: protocol.Position{Line: 1, Character: 4}}})
	if err != nil || h == nil {
		t.Fatalf("hover: %v %+v", err, h)
	}
	if got := h.Contents.(*protocol.MarkupContent).Value; got != "```kigumi\nlocal N: usize\n```" {
		t.Fatalf("const param hover: %q", got)
	}
}

// initParams announces root the way current clients do, through
// workspaceFolders rather than the deprecated rootUri.
func initParams(root uri.URI) *protocol.InitializeParams {
	p := &protocol.InitializeParams{}
	p.WorkspaceFolders = protocol.NewNullable([]protocol.WorkspaceFolder{{URI: root, Name: "root"}})
	return p
}
