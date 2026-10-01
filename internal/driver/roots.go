package driver

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// IsModuleRoot reports whether dir is a module: it holds a mod.kg manifest
// or a cmd/ directory. A module inside another one is loaded on its own.
func IsModuleRoot(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ManifestName)); err == nil {
		return true
	}
	st, err := os.Stat(filepath.Join(dir, "cmd"))
	return err == nil && st.IsDir()
}

// FindModuleRoot walks up from dir to the nearest module root; ok is false
// when none is found before the file system root.
func FindModuleRoot(dir string) (string, bool) {
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		if IsModuleRoot(d) {
			return d, true
		}
		if filepath.Dir(d) == d {
			return "", false
		}
	}
}

// ModuleRoots lists workspace and every nested module root below it.
func ModuleRoots(workspace, stdRoot string) []string {
	roots := []string{workspace}
	filepath.WalkDir(workspace, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == workspace {
			return nil
		}
		if skipDir(path, stdRoot) {
			return fs.SkipDir
		}
		if IsModuleRoot(path) {
			roots = append(roots, path)
		}
		return nil
	})
	return roots
}

// skipDir excludes hidden directories, node_modules and the std root from
// a module walk.
func skipDir(path, stdRoot string) bool {
	name := filepath.Base(path)
	return strings.HasPrefix(name, ".") || name == "node_modules" || sameDir(path, stdRoot)
}

func sameDir(a, b string) bool {
	if b == "" {
		return false
	}
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return aa == bb
}
