package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

func TestCompareVersions(t *testing.T) {
	t.Parallel()
	order := []string{"v0.9.0", "v1.0.0-alpha", "v1.0.0-alpha.1", "v1.0.0-alpha.beta", "v1.0.0-beta", "v1.0.0-beta.2", "v1.0.0-beta.11", "v1.0.0-rc.1", "v1.0.0", "v1.0.0+build", "v1.0.1", "v1.10.0", "v2.0.0"}
	for i := 1; i < len(order); i++ {
		a, b := order[i-1], order[i]
		want := -1
		if strings.HasPrefix(b, a+"+") {
			want = 0
		}
		if got := driver.CompareVersions(a, b); got != want {
			t.Errorf("Compare(%s, %s) = %d, want %d", a, b, got, want)
		}
	}
	for _, v := range []string{"v1.0.0", "v1.0.0-alpha.1+meta", "0123456abcdef"} {
		if err := driver.ValidVersion(v); err != nil {
			t.Errorf("%s should be valid: %v", v, err)
		}
	}
}

// Two numeric prerelease identifiers that overflow int64 must still
// compare by magnitude, not collapse to equal via silent ParseInt
// saturation.
func TestCompareVersionsPrereleaseOverflow(t *testing.T) {
	small := "v1.0.0-99999999999999999999"
	big := "v1.0.0-888888888888888888888888"
	if got := driver.CompareVersions(small, big); got != -1 {
		t.Errorf("Compare(%s, %s) = %d, want -1", small, big, got)
	}
	if got := driver.CompareVersions(big, small); got != 1 {
		t.Errorf("Compare(%s, %s) = %d, want 1", big, small, got)
	}
}

// writeModule lays out a module with a manifest, for selection tests.
func writeModule(t *testing.T, dir, manifest string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "mod.kg"), []byte(manifest), 0o644)
	os.WriteFile(filepath.Join(dir, "lib.kg"), []byte("pub fn v() -> Int {\n    1\n}\n"), 0o644)
}

func manifestFor(name string, requires ...string) string {
	s := "Module {\n    name: \"" + name + "\"\n    kigumi: \"0.1\"\n}\n"
	for i := 0; i+1 < len(requires); i += 2 {
		s += "Require { name: \"" + requires[i] + "\", version: \"" + requires[i+1] + "\" }\n"
	}
	return s
}

// A manifest cycle is an error with the chain, whether or not any package
// imports across it; mixing two majors of one module is refused.
func TestSelectionRejectsCycleAndMajorMix(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeModule(t, filepath.Join(root, "a"), manifestFor("example.com/a", "example.com/b", "v1.0.0"))
	writeModule(t, filepath.Join(root, "b"), manifestFor("example.com/b", "example.com/a", "v1.0.0"))
	writeModule(t, filepath.Join(root, "c"), manifestFor("example.com/c", "example.com/x", "v2.0.0"))
	writeModule(t, filepath.Join(root, "x"), manifestFor("example.com/x"))
	local := driver.Local{Replaces: map[string]string{
		"example.com/a": filepath.Join(root, "a"), "example.com/b": filepath.Join(root, "b"),
		"example.com/c": filepath.Join(root, "c"), "example.com/x": filepath.Join(root, "x"),
	}}
	app := driver.ModFile{Name: "example.com/app", Requires: []driver.Require{{Name: "example.com/a", Version: "v1.0.0"}}}
	if _, err := driver.SelectVersions(app, local); err == nil || !strings.Contains(err.Error(), "dependency cycle: example.com/a -> example.com/b -> example.com/a") {
		t.Fatalf("cycle: %v", err)
	}
	mixed := driver.ModFile{Name: "example.com/app", Requires: []driver.Require{{Name: "example.com/x", Version: "v1.2.0"}, {Name: "example.com/c", Version: "v1.0.0"}}}
	if _, err := driver.SelectVersions(mixed, local); err == nil || !strings.Contains(err.Error(), "different major versions cannot be mixed") {
		t.Fatalf("major mix: %v", err)
	}
}
