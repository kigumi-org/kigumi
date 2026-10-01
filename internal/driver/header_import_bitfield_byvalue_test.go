package driver_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// TestAotHeaderImportBitfieldByValue: a
// bitfield struct zig demotes to an opaque type is ABI-safe only behind a
// pointer, so the importer must skip a by-value function using it (E994)
// while still importing the header's other functions.
func TestAotHeaderImportBitfieldByValue(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	if _, err := exec.LookPath("zig"); err != nil {
		t.Skip("no zig")
	}
	root, err := filepath.Abs("../../testdata/cheader_bitfield_byvalue")
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
	if m.HasParseErrors() {
		t.Fatalf("importing the header must not fail the whole module:\n%s", m.Diagnostics())
	}
	d := m.Diagnostics()
	for _, want := range []string{"E994", "bitfield_byvalue_probe", "BitfieldThing", "by value"} {
		if !strings.Contains(d, want) {
			t.Errorf("skip diagnostic missing %q:\n%s", want, d)
		}
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
	if want := "7\n"; string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
