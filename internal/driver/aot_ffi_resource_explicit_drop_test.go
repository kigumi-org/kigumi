package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kigumi/internal/driver"
)

// TestAotFFIResourceExplicitDrop: `ffi.CString.drop`
// needs real foreign memory, so unlike `ffi.CValue` it never runs on the
// interpreter or the VM (internal/interp/builtin_ffi.go). Native is the
// only engine that can catch its double free, and KIGUMI_RC_CHECK makes
// glibc's tcache detector fatal instead of merely likely.
func TestAotFFIResourceExplicitDrop(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	t.Setenv("KIGUMI_CFLAGS", "-DKIGUMI_RC_CHECK")
	root := t.TempDir()
	src := `import ffi from std/ffi

let text = "hello"
let cs = ffi.CString.new(&text).unwrap()
print "${ffi.isNull(cs.ptr())}"
cs.drop()
print "done"
`
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"cstringdrop\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "prog")
	var buildErr bytes.Buffer
	ok, err := testBuild(m, exe, &buildErr)
	if err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}
	out, err := exec.Command(exe).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if want := "false\ndone\n"; string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
