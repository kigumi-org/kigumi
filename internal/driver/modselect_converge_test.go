package driver_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

func putManifest(t *testing.T, name, version, rest string) {
	t.Helper()
	dir, err := driver.CacheDir(driver.Require{Name: name, Version: version})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "mod.kg"), manifest(name, rest), 0o644)
	os.WriteFile(filepath.Join(dir, driver.OriginFileName), []byte("cafebabe\n"), 0o644)
}

// TestSelectVersionsOrderIndependent covers MOD-001: selection must not
// depend on the order the root lists its Requires in, and a module
// superseded before its manifest is read must not leak its own requires
// into the result.
func TestSelectVersionsOrderIndependent(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)

	putManifest(t, "p", "v1.0.0", "Require { name: \"m\", version: \"v1.1.0\" }\n")
	putManifest(t, "q", "v1.0.0", "Require { name: \"r\", version: \"v1.0.0\" }\n")
	putManifest(t, "r", "v1.0.0", "Require { name: \"m\", version: \"v1.9.0\" }\n")
	putManifest(t, "m", "v1.1.0", "Require { name: \"z\", version: \"v3.0.0\" }\n")
	putManifest(t, "m", "v1.9.0", "")
	putManifest(t, "z", "v3.0.0", "")

	pFirst := driver.ModFile{Requires: []driver.Require{
		{Name: "p", Version: "v1.0.0"},
		{Name: "q", Version: "v1.0.0"},
	}}
	qFirst := driver.ModFile{Requires: []driver.Require{
		{Name: "q", Version: "v1.0.0"},
		{Name: "p", Version: "v1.0.0"},
	}}

	selA, err := driver.SelectVersions(pFirst, driver.Local{})
	if err != nil {
		t.Fatal(err)
	}
	selB, err := driver.SelectVersions(qFirst, driver.Local{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selA.Requires, selB.Requires) {
		t.Fatalf("selection depends on root order:\n p,q -> %+v\n q,p -> %+v", selA.Requires, selB.Requires)
	}
	if selA.Requires["m"].Version != "v1.9.0" {
		t.Fatalf("m should resolve to the newer v1.9.0, got %+v", selA.Requires["m"])
	}
	if _, ok := selA.Requires["z"]; ok {
		t.Fatalf("z is only required by the superseded m v1.1.0 and must not survive: %+v", selA.Requires)
	}
}

// Two Requires for the same module at build-metadata-only-different tags
// (equal SemVer precedence, different literal string) must conflict, not
// silently pick whichever was discovered first: CacheDir keys off the
// literal tag, so the two can address different cache trees.
func TestSelectVersionsBuildMetadataConflict(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)

	root := driver.ModFile{Requires: []driver.Require{
		{Name: "b", Version: "v1.0.0+buildA"},
		{Name: "b", Version: "v1.0.0+buildB"},
	}}
	_, err := driver.SelectVersions(root, driver.Local{})
	if err == nil || !strings.Contains(err.Error(), "v1.0.0+buildA") || !strings.Contains(err.Error(), "v1.0.0+buildB") {
		t.Fatalf("build-metadata-only-different tags should conflict, naming both: %v", err)
	}
}

// TestSelectVersionsPrunesSupersededOnlyDep covers MOD-003: a dependency
// reachable only through a require of a now-superseded manifest (not the
// winning version) must be dropped from the selection, not merely
// unreachable via the winner's own edges.
func TestSelectVersionsPrunesSupersededOnlyDep(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)

	putManifest(t, "a", "v1.0.0", "Require { name: \"b\", version: \"v1.0.0\" }\n")
	putManifest(t, "a", "v1.1.0", "")
	putManifest(t, "b", "v1.0.0", "")
	putManifest(t, "c", "v1.0.0", "Require { name: \"a\", version: \"v1.1.0\" }\n")

	root := driver.ModFile{Requires: []driver.Require{
		{Name: "a", Version: "v1.0.0"},
		{Name: "c", Version: "v1.0.0"},
	}}
	sel, err := driver.SelectVersions(root, driver.Local{})
	if err != nil {
		t.Fatal(err)
	}
	if sel.Requires["a"].Version != "v1.1.0" {
		t.Fatalf("a should converge to v1.1.0 (required by c): %+v", sel.Requires["a"])
	}
	if _, ok := sel.Requires["b"]; ok {
		t.Fatalf("b is required only by the superseded a v1.0.0 and must not survive: %+v", sel.Requires)
	}
}
