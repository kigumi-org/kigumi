package driver

import (
	"io/fs"
	"path/filepath"
	"strings"

	"kigumi/internal/token"
)

// loadTree reads every directory under root as a package; skip prunes
// subtrees that belong to another module.
func (m *Module) loadTree(root, prefix string, std, test bool, skip func(string) bool) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && (strings.HasPrefix(d.Name(), ".") || skip != nil && skip(path)) {
			return fs.SkipDir
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		pkgPath := filepath.ToSlash(rel)
		if pkgPath == "." {
			pkgPath = ""
		}
		fileRel := pkgPath
		if prefix != "" {
			pkgPath = strings.TrimSuffix(prefix+"/"+pkgPath, "/")
		}
		// The root module's file names stay root-relative so diagnostics
		// and editors find them on disk; dependencies keep the prefix.
		if root != m.Root {
			fileRel = pkgPath
		}
		return m.loadPackage(path, pkgPath, fileRel, std, test)
	})
}

func (m *Module) fileStore() *token.SourceStore {
	if m.FileStore == nil {
		m.FileStore = &token.SourceStore{}
	}
	return m.FileStore
}
