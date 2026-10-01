package lsp

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"kigumi/internal/diag"
	"kigumi/internal/driver"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// snapshot is one analysis of the module containing a document.
type snapshot struct {
	root string
	mod  *driver.Module
	res  *sem.Result
	err  error
}

func (s *Server) loadDoc(ctx context.Context, u uri.URI) *snapshot {
	return s.load(ctx, s.moduleRoot(uriToPath(u)), u)
}

func (s *Server) load(ctx context.Context, root string, u uri.URI) *snapshot {
	path := ""
	if u != "" {
		path = uriToPath(u)
	}
	key := root + "\x00" + path
	if snap, ok := s.snaps[key]; ok {
		return snap
	}
	snap := s.loadFresh(ctx, root, u, path)
	s.snaps[key] = snap
	return snap
}

func (s *Server) loadFresh(ctx context.Context, root string, u uri.URI, path string) *snapshot {
	overlay := map[string][]byte{}
	for d, text := range s.docs {
		overlay[uriToPath(d)] = []byte(text)
	}
	mod, err := driver.LoadModule(root, driver.LoadOptions{Test: true, StdRoot: s.stdRoot, Overlay: overlay})
	if err == nil && mod.Entry == "" {
		prefer, _ := filepath.Rel(root, path)
		mod.Entry = pickEntry(mod, filepath.ToSlash(prefer))
	}
	snap := &snapshot{root: root, mod: mod, err: err}
	if err != nil {
		s.logf(ctx, protocol.MessageTypeError, "load module %s: %v", root, err)
		return snap
	}
	// Edited files rarely fully parse; recovering here keeps hover,
	// completion and definition working around the broken statement.
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				snap.res = nil
				s.logf(ctx, protocol.MessageTypeError, "checker panicked on %s: %v", path, rec)
			}
		}()
		snap.res, snap.err = driver.CheckOnly(mod)
	}()
	if snap.err != nil {
		s.logf(ctx, protocol.MessageTypeError, "check %s: %v", root, snap.err)
	}
	return snap
}

// Analysis runs synchronously under s.mu, so publish needs no staleness check.
func (s *Server) publish(ctx context.Context, u uri.URI, ds []protocol.Diagnostic) {
	if ds == nil {
		ds = []protocol.Diagnostic{}
	}
	if client, ok := protocol.ClientFromContext(ctx); ok {
		p := &protocol.PublishDiagnosticsParams{URI: u, Diagnostics: ds}
		if v, ok := s.docVersions[u]; ok {
			p.Version = protocol.NewOptional(v)
		}
		client.PublishDiagnostics(ctx, p)
	}
}

func (s *Server) analyze(ctx context.Context, u uri.URI) {
	path := uriToPath(u)
	if driver.IsManifest(filepath.Base(path)) {
		s.publish(ctx, u, s.manifestDiagnostics(u))
	}
	s.analyzeModule(ctx, s.moduleRoot(path), u)
}

func (s *Server) analyzeModule(ctx context.Context, root string, u uri.URI) {
	snap := s.load(ctx, root, u)
	if snap.mod == nil {
		if u != "" && !driver.IsManifest(filepath.Base(uriToPath(u))) {
			s.publish(ctx, u, []protocol.Diagnostic{{Message: protocol.String(snap.err.Error()), Severity: protocol.DiagnosticSeverityError}})
		}
		return
	}
	fresh, stale := map[uri.URI]bool{}, s.published[root]
	for _, p := range snap.mod.Order {
		pkg := snap.mod.Packages[p]
		if pkg.Std {
			continue
		}
		for _, t := range pkg.Files {
			fileURI := uri.File(filepath.Join(snap.root, filepath.FromSlash(t.File.Name)))
			all := append([]diag.Diagnostic{}, t.Diags...)
			if snap.res != nil {
				all = append(all, snap.res.Diagnostics(t)...)
			}
			var ds []protocol.Diagnostic
			for _, d := range all {
				ds = append(ds, s.toDiagnostic(snap, t.File, d))
			}
			if len(ds) > 0 || stale[fileURI] || fileURI == u {
				s.publish(ctx, fileURI, ds)
			}
			fresh[fileURI] = len(ds) > 0
		}
	}
	for d := range stale {
		if !fresh[d] {
			s.publish(ctx, d, nil)
		}
	}
	s.published[root] = fresh
	s.logf(ctx, protocol.MessageTypeLog, "analyzed %s: %d packages, %d files with diagnostics", snap.root, len(snap.mod.Order), countTrue(fresh))
}

// pickEntry treats the one script-shaped file outside cmd/ as the entry,
// or the preferred (open) file when several qualify; the CLI stays strict.
func pickEntry(mod *driver.Module, prefer string) string {
	var found []string
	for _, p := range mod.Order {
		pkg := mod.Packages[p]
		if pkg.Std || strings.HasPrefix(mod.Rel(p), "cmd/") {
			continue
		}
		for _, t := range pkg.Files {
			if t.HasTopLevelStatements() || t.HasExplicitMain() {
				found = append(found, t.File.Name)
			}
		}
	}
	if len(found) == 1 {
		return found[0]
	}
	if slices.Contains(found, prefer) {
		return prefer
	}
	return ""
}

func countTrue(m map[uri.URI]bool) int {
	n := 0
	for _, v := range m {
		if v {
			n++
		}
	}
	return n
}

func (snap *snapshot) fileOf(path string) *syntax.Tree {
	rel, err := filepath.Rel(snap.root, path)
	if err != nil {
		return nil
	}
	for _, p := range snap.mod.Packages {
		for _, t := range p.Files {
			if t.File.Name == filepath.ToSlash(rel) {
				return t
			}
		}
	}
	return nil
}

func (s *Server) pathOfFile(snap *snapshot, name string) string {
	if rest, ok := strings.CutPrefix(name, "std/"); ok {
		return filepath.Join(s.stdRoot, filepath.FromSlash(rest))
	}
	return filepath.Join(snap.root, filepath.FromSlash(name))
}
