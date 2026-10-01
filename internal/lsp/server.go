// Package lsp implements the Kigumi language server on go.lsp.dev/protocol:
// diagnostics, hover, go to definition, formatting, document symbols and
// completion, computed from the driver and the checker.
package lsp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"kigumi/internal/driver"
)

// Server holds the open documents and the module roots; every LSP method
// it does not override answers "method not found". go.lsp.dev runs
// handlers concurrently, so each one takes mu.
type Server struct {
	protocol.UnimplementedServer
	mu          sync.Mutex
	root        string
	stdRoot     string
	docs        map[uri.URI]string
	docVersions map[uri.URI]int32
	published   map[string]map[uri.URI]bool
	// snaps caches one analysis per module root and entry until a
	// document or the disk changes, so hover, completion and the rest do
	// not each re-check the module.
	snaps map[string]*snapshot
}

func New(stdRoot string) *Server {
	return &Server{stdRoot: stdRoot, docs: map[uri.URI]string{}, docVersions: map[uri.URI]int32{}, published: map[string]map[uri.URI]bool{}, snaps: map[string]*snapshot{}}
}

func (s *Server) invalidate() { s.snaps = map[string]*snapshot{} }

type stdio struct{}

func (stdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (stdio) Close() error                { return os.Stdout.Close() }

// Serve speaks the protocol on stdin/stdout until the client hangs up.
func Serve(stdRoot string) error {
	_, conn, _ := protocol.NewServer(context.Background(), New(stdRoot), jsonrpc2.NewStream(stdio{}))
	<-conn.Done()
	if err := conn.Err(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

func (s *Server) Initialize(ctx context.Context, p *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if folders, ok := p.WorkspaceFolders.Get(); ok && len(folders) > 0 {
		s.root = uriToPath(folders[0].URI)
	}
	var opts struct {
		StdRoot string `json:"stdRoot"`
	}
	if json.Unmarshal([]byte(p.InitializationOptions), &opts) == nil && opts.StdRoot != "" {
		s.stdRoot = opts.StdRoot
	}
	if s.stdRoot == "" {
		s.stdRoot = findStd(s.root)
	}
	// Locations are file URIs, so a std root given relative to the cwd
	// (`kigumi lsp` finds ./std) must become absolute.
	if abs, err := filepath.Abs(s.stdRoot); s.stdRoot != "" && err == nil {
		s.stdRoot = abs
	}
	s.logf(ctx, protocol.MessageTypeInfo, "kigumi lsp: workspace %q, std %q", s.root, s.stdRoot)
	if s.stdRoot == "" {
		s.logf(ctx, protocol.MessageTypeWarning, "std stubs not found: set kigumi.stdRoot or $KIGUMI_STD, or keep std/ in the workspace; std imports will fail to resolve")
	}
	prepareRename := true
	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync:                protocol.TextDocumentSyncKindFull,
			HoverProvider:                   protocol.Boolean(true),
			DefinitionProvider:              protocol.Boolean(true),
			ImplementationProvider:          protocol.Boolean(true),
			ReferencesProvider:              protocol.Boolean(true),
			WorkspaceSymbolProvider:         protocol.Boolean(true),
			CodeActionProvider:              protocol.Boolean(true),
			RenameProvider:                  &protocol.RenameOptions{PrepareProvider: &prepareRename},
			DocumentFormattingProvider:      protocol.Boolean(true),
			DocumentRangeFormattingProvider: protocol.Boolean(true),
			DocumentSymbolProvider:          protocol.Boolean(true),
			CompletionProvider:              &protocol.CompletionOptions{TriggerCharacters: []string{"."}},
			SignatureHelpProvider:           &protocol.SignatureHelpOptions{TriggerCharacters: []string{"(", ","}},
			DocumentHighlightProvider:       protocol.Boolean(true),
			FoldingRangeProvider:            protocol.Boolean(true),
		},
		ServerInfo: protocol.ServerInfo{Name: "kigumi", Version: protocol.NewOptional("0.1")},
	}, nil
}

// Initialized analyzes the whole workspace so every file gets diagnostics
// before it is opened; DidChangeWatchedFiles repeats that on disk changes.
func (s *Server) Initialized(ctx context.Context, _ *protocol.InitializedParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.analyzeWorkspace(ctx)
	return nil
}

func (s *Server) DidChangeWatchedFiles(ctx context.Context, _ *protocol.DidChangeWatchedFilesParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invalidate()
	s.analyzeWorkspace(ctx)
	return nil
}

func (s *Server) analyzeWorkspace(ctx context.Context) {
	if s.root == "" {
		return
	}
	for _, root := range driver.ModuleRoots(s.root, s.stdRoot) {
		s.analyzeModule(ctx, root, "")
	}
}

func (s *Server) Shutdown(context.Context) error                           { return nil }
func (s *Server) Exit(context.Context) error                               { return nil }
func (s *Server) SetTrace(context.Context, *protocol.SetTraceParams) error { return nil }

func (s *Server) DidOpen(ctx context.Context, p *protocol.DidOpenTextDocumentParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[p.TextDocument.URI] = p.TextDocument.Text
	s.docVersions[p.TextDocument.URI] = p.TextDocument.Version
	s.invalidate()
	s.analyze(ctx, p.TextDocument.URI)
	return nil
}

func (s *Server) DidChange(ctx context.Context, p *protocol.DidChangeTextDocumentParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := p.TextDocument.URI
	s.docVersions[u] = p.TextDocument.Version
	for _, change := range p.ContentChanges {
		switch c := change.(type) {
		case *protocol.TextDocumentContentChangeWholeDocument:
			s.docs[u] = c.Text
		case *protocol.TextDocumentContentChangePartial:
			text := s.docs[u]
			f := fileOfText(text)
			start, end := offset(f, c.Range.Start), offset(f, c.Range.End)
			s.docs[u] = text[:start] + c.Text + text[end:]
		}
	}
	s.invalidate()
	s.analyze(ctx, u)
	return nil
}

func (s *Server) DidClose(ctx context.Context, p *protocol.DidCloseTextDocumentParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, p.TextDocument.URI)
	delete(s.docVersions, p.TextDocument.URI)
	s.invalidate()
	s.analyze(ctx, p.TextDocument.URI)
	return nil
}
