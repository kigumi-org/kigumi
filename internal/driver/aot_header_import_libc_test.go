package driver_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"kigumi/internal/driver"
)

// TestAotHeaderImportLibc imports the real system `string.h` and calls
// `strlen` through the generated declaration; native
// only, per the same FFI convention as TestAotFFI.
func TestAotHeaderImportLibc(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	if _, err := exec.LookPath("zig"); err != nil {
		t.Skip("no zig")
	}
	root, err := filepath.Abs("../../testdata/cheader_libc")
	if err != nil {
		t.Fatal(err)
	}
	std, err := filepath.Abs("../../std")
	if err != nil {
		t.Fatal(err)
	}
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
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
	if want := "5\n"; string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
