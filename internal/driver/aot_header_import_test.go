package driver_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"kigumi/internal/driver"
)

// TestAotHeaderImport covers its own fixture: testdata/cheader/sample.h
// declares a struct, an enum, three functions (one taking and returning the
// enum, to keep its members' type pinned to the enum's own) and an integer
// macro; mod.kg's `Header` record imports it as `c/sample`, and helper.c
// (via KIGUMI_CFLAGS, the same convention TestAotFFI uses) supplies the
// definitions. Native is the only engine that can link it, so unlike
// testdata/run this is a Go test rather than a txtar fixture.
func TestAotHeaderImport(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	if _, err := exec.LookPath("zig"); err != nil {
		t.Skip("no zig")
	}
	root, err := filepath.Abs("../../testdata/cheader")
	if err != nil {
		t.Fatal(err)
	}
	std, err := filepath.Abs("../../std")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("KIGUMI_CFLAGS", filepath.Join(root, "helper.c"))
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	if d := m.Diagnostics(); d != "" {
		t.Fatalf("unexpected diagnostics:\n%s", d)
	}
	exe := filepath.Join(t.TempDir(), "prog")
	var buildErr bytes.Buffer
	ok, err := testBuild(m, exe, &buildErr)
	if err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}
	out, err := exec.Command(exe).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	if want := "3 7 0 1 2 64 1\n"; string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
