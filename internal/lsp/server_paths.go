package lsp

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"go.lsp.dev/uri"

	"kigumi/internal/driver"
)

// findStd locates the std stubs when the CLI passed none: <root>/std, or
// next to the executable.
func findStd(root string) string {
	candidates := []string{filepath.Join(root, "std")}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "std"), filepath.Join(filepath.Dir(exe), "..", "std"))
	}
	for _, c := range candidates {
		if st, err := os.Stat(filepath.Join(c, "prelude")); err == nil && st.IsDir() {
			return c
		}
	}
	return ""
}

func uriToPath(u uri.URI) string {
	parsed, err := url.Parse(string(u))
	if err != nil || parsed.Scheme != "file" {
		return string(u)
	}
	return filepath.FromSlash(parsed.Path)
}

// moduleRoot picks the module root for a document: inside the workspace,
// the nearest ancestor holding cmd/ or else the workspace itself; outside,
// the nearest ancestor module, or else the file's own directory (script
// mode).
func (s *Server) moduleRoot(path string) string {
	if s.root != "" && strings.HasPrefix(path, s.root+string(filepath.Separator)) {
		for dir := filepath.Dir(path); dir != s.root; dir = filepath.Dir(dir) {
			if driver.IsModuleRoot(dir) {
				return dir
			}
		}
		return s.root
	}
	if root, ok := driver.FindModuleRoot(filepath.Dir(path)); ok {
		return root
	}
	return filepath.Dir(path)
}
