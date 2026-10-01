package lsp

import (
	"path/filepath"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"kigumi/internal/diag"
	"kigumi/internal/driver"
	"kigumi/internal/token"
)

// manifestDiagnostics validates an open mod.kg or mod.lock.kg buffer. The
// module itself is still loaded from the saved file, so a broken manifest
// shows its own errors while the last good one keeps dependencies resolved.
func (s *Server) manifestDiagnostics(u uri.URI) []protocol.Diagnostic {
	path := uriToPath(u)
	f := token.NewFile(filepath.Base(path), []byte(s.docs[u]))
	var ds []diag.Diagnostic
	switch {
	case f.Name == driver.LocalName:
		_, ds = driver.ParseLocal(f, filepath.Dir(path))
	case strings.HasSuffix(f.Name, ".lock.kg"):
		_, ds = driver.ParseLock(f)
	default:
		_, ds = driver.ParseModFile(f)
	}
	out := []protocol.Diagnostic{}
	for _, d := range ds {
		out = append(out, s.toDiagnostic(nil, f, d))
	}
	return out
}
