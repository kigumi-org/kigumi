package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// TestBuildProgramRootsFollowSelection covers MOD-002/MOD-004: Roots must
// list the directory minimal version selection resolved, not the root
// manifest's raw (possibly superseded) Require, and must include a
// dependency that only the graph, not the root's own manifest, requires.
func TestBuildProgramRootsFollowSelection(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	lock := driver.Lock{Entries: map[string]driver.LockEntry{}}
	put := func(name, version, rest string) {
		dir, _ := driver.CacheDir(driver.Require{Name: name, Version: version})
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "mod.kg"), manifest(name, rest), 0o644)
		os.WriteFile(filepath.Join(dir, driver.OriginFileName), []byte("cafebabe\n"), 0o644)
		hash, err := driver.TreeHash(dir)
		if err != nil {
			t.Fatal(err)
		}
		lock.Entries[name] = driver.LockEntry{Version: version, Commit: "cafebabe", Hash: hash}
	}
	put("github.com/acme/c", "v1.0.0", "")
	put("github.com/acme/c", "v1.1.0", "")
	put("github.com/acme/d", "v1.0.0", "")
	put("github.com/acme/b", "v1.0.0", "Require { name: \"github.com/acme/c\", version: \"v1.1.0\" }\nRequire { name: \"github.com/acme/d\", version: \"v1.0.0\" }\n")

	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), manifest("app", "Require { name: \"github.com/acme/b\", version: \"v1.0.0\" }\nRequire { name: \"github.com/acme/c\", version: \"v1.0.0\" }\n"), 0o644)
	if err := driver.WriteLock(root, lock); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "build.kg"), []byte("import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {}\n"), 0o644)
	std, _ := filepath.Abs("../../std")
	p, ok, err := driver.LoadBuildProgram(root, driver.LoadOptions{StdRoot: std})
	if err != nil || !ok {
		t.Fatalf("load: %v %v", ok, err)
	}
	// app's own mod.kg asks for c v1.0.0, but b needs c v1.1.0; selection
	// must resolve to the newer one.
	if p.Module.Selected["github.com/acme/c"].Version != "v1.1.0" {
		t.Fatalf("selection: %+v", p.Module.Selected)
	}
	roots, err := p.Roots(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	selectedC, _ := driver.CacheDir(driver.Require{Name: "github.com/acme/c", Version: "v1.1.0"})
	staleC, _ := driver.CacheDir(driver.Require{Name: "github.com/acme/c", Version: "v1.0.0"})
	transitiveD, _ := driver.CacheDir(driver.Require{Name: "github.com/acme/d", Version: "v1.0.0"})
	var hasSelectedC, hasStaleC, hasD bool
	for _, r := range roots {
		switch r {
		case selectedC:
			hasSelectedC = true
		case staleC:
			hasStaleC = true
		case transitiveD:
			hasD = true
		}
	}
	if !hasSelectedC {
		t.Fatalf("roots miss the selected version %s: %v", selectedC, roots)
	}
	if hasStaleC {
		t.Fatalf("roots keep the stale unselected version %s: %v", staleC, roots)
	}
	if !hasD {
		t.Fatalf("roots miss a dependency required only transitively: %s: %v", transitiveD, roots)
	}
}

// TestSelectVersionsOriginConflict covers MOD-005: the same name and
// version from two different origins is a conflict, not a silent merge.
func TestSelectVersionsOriginConflict(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	dir, _ := driver.CacheDir(driver.Require{Name: "foo", Version: "v1.0.0"})
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "mod.kg"), manifest("foo", ""), 0o644)
	os.WriteFile(filepath.Join(dir, driver.OriginFileName), []byte("cafebabe\n"), 0o644)

	conflicting := driver.ModFile{Requires: []driver.Require{
		{Name: "foo", Version: "v1.0.0", URL: "https://origin-a.example/foo.git"},
		{Name: "foo", Version: "v1.0.0", URL: "https://origin-b.example/foo.git"},
	}}
	if _, err := driver.SelectVersions(conflicting, driver.Local{}); err == nil || !strings.Contains(err.Error(), "origin-a.example") || !strings.Contains(err.Error(), "origin-b.example") {
		t.Fatalf("same name+version from different origins should conflict: %v", err)
	}

	same := driver.ModFile{Requires: []driver.Require{
		{Name: "foo", Version: "v1.0.0", URL: "https://origin-a.example/foo.git"},
		{Name: "foo", Version: "v1.0.0", URL: "https://origin-a.example/foo.git"},
	}}
	sel, err := driver.SelectVersions(same, driver.Local{})
	if err != nil || sel.Requires["foo"].URL != "https://origin-a.example/foo.git" {
		t.Fatalf("identical origins should not conflict: %+v %v", sel, err)
	}
}
