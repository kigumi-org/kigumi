package driver

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func (m *Module) loadPackage(dir, pkgPath, fileRel string, std, test bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var files []*syntax.Tree
	names := map[string]bool{}
	// build.kg is the build program only at the module root; a package may
	// still have a file of that name (std/build does).
	isProgram := func(name string) bool { return name == BuildFileName && sameDir(dir, m.Root) }
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".kg") && !IsManifest(e.Name()) && !isProgram(e.Name()) {
			names[e.Name()] = true
		}
	}
	for abs := range m.overlay {
		if filepath.Dir(abs) == filepath.Clean(dir) && strings.HasSuffix(abs, ".kg") && !IsManifest(filepath.Base(abs)) && !isProgram(filepath.Base(abs)) {
			names[filepath.Base(abs)] = true
		}
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	for _, name := range sorted {
		if strings.HasSuffix(name, "_test.kg") && !test {
			continue
		}
		if tagged, ok := m.platformFile(name); tagged && !ok {
			continue
		}
		full := filepath.Join(dir, name)
		src, ok := m.overlay[full]
		if !ok {
			var err error
			if src, err = os.ReadFile(full); err != nil {
				return err
			}
		}
		// Root-relative names keep golden output independent of the checkout path.
		fileName := name
		if fileRel != "" {
			fileName = fileRel + "/" + name
		}
		f := token.NewFile(fileName, src)
		m.fileStore().Add(f)
		files = append(files, syntax.Parse(f))
	}
	if len(files) == 0 {
		return nil
	}
	// Files at the root of a manifest-less directory form package main:
	// the entry of a single-script module, which nothing needs to import.
	if pkgPath == "" {
		pkgPath = "main"
	}
	if _, dup := m.Packages[pkgPath]; dup {
		return fmt.Errorf("package %q is defined by two directories", pkgPath)
	}
	m.Packages[pkgPath] = &Package{Path: pkgPath, Dir: dir, Files: files, Std: std}
	return nil
}

// Diagnostics renders every parse diagnostic in the module, in package order.
