package driver

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// dependencyOverlap reports the first package path a dependency's tree
// would produce that some already-loaded package (the root module's own
// tree, or an earlier dependency) already claims, "" when none does. A
// module name being a segment-wise prefix of another's is not itself an
// overlap: two modules with disjoint directory trees never produce
// the same package path, no matter how their names nest.
func (m *Module) dependencyOverlap(depDir, prefix string, skip func(string) bool) string {
	found := ""
	filepath.WalkDir(depDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != depDir && (strings.HasPrefix(d.Name(), ".") || skip(path)) {
			return fs.SkipDir
		}
		rel, err := filepath.Rel(depDir, path)
		if err != nil {
			return err
		}
		pkgPath := filepath.ToSlash(rel)
		if pkgPath == "." {
			pkgPath = ""
		}
		if prefix != "" {
			pkgPath = strings.TrimSuffix(prefix+"/"+pkgPath, "/")
		}
		if _, exists := m.Packages[pkgPath]; !exists || !m.definesPackage(path) {
			return nil
		}
		found = pkgPath
		return fs.SkipAll
	})
	return found
}

// definesPackage mirrors loadPackage's file filter: a manifest-only or
// empty directory produces no package and cannot collide with one.
func (m *Module) definesPackage(dir string) bool {
	isProgram := func(name string) bool { return name == BuildFileName && sameDir(dir, m.Root) }
	packageFile := func(name string) bool {
		return strings.HasSuffix(name, ".kg") && !IsManifest(name) && !isProgram(name) &&
			!strings.HasSuffix(name, "_test.kg")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || !packageFile(e.Name()) {
			continue
		}
		if tagged, ok := m.platformFile(e.Name()); tagged && !ok {
			continue
		}
		return true
	}
	for abs := range m.overlay {
		if filepath.Dir(abs) != filepath.Clean(dir) || !packageFile(filepath.Base(abs)) {
			continue
		}
		if tagged, ok := m.platformFile(filepath.Base(abs)); tagged && !ok {
			continue
		}
		return true
	}
	return false
}
