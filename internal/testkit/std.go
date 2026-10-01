// Package testkit holds helpers shared by the test packages: loading the
// std sources the way the driver does, without going through a module.
package testkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/platform"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// LoadStd parses every std package under root for the host platform.
func LoadStd(t testing.TB, root string) []*sem.Package {
	t.Helper()
	return load(t, root, "")
}

// LoadPrelude parses only std/prelude, for tests that want a small universe.
func LoadPrelude(t testing.TB, root string) []*sem.Package {
	t.Helper()
	return load(t, root, "std/prelude")
}

func load(t testing.TB, root, only string) []*sem.Package {
	var out []*sem.Package
	byDir := map[string]*sem.Package{}
	hostOS, hostArch := platform.Host()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".kg") {
			return err
		}
		if tagged, ok := platform.File(d.Name(), hostOS, hostArch); tagged && !ok {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.Join("std", rel)
		dir := filepath.ToSlash(filepath.Dir(rel))
		if only != "" && dir != only {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		tree := syntax.Parse(token.NewFile(name, src))
		if tree.HasErrors() {
			t.Fatalf("std source %s has parse errors:\n%s", name, tree.Dump())
		}
		p := byDir[dir]
		if p == nil {
			p = &sem.Package{Path: dir, Module: "std", Std: true}
			byDir[dir] = p
			out = append(out, p)
		}
		p.Files = append(p.Files, tree)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
