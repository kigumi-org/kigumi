package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kigumi/internal/driver"
)

// TestDLGetNullValuedSymbol reproduces dlsym-null: a symbol that legitimately
// resolves to address 0 must not be reported as a lookup failure. POSIX
// dlsym can return NULL for a symbol that exists, so the only correct check
// is dlerror before and after, not a NULL check on the result.
func TestDLGetNullValuedSymbol(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("no cc")
	}
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root := t.TempDir()
	const src = `import dl from std/dl

type VoidFn = extern(C) fn() -> Unit

fn run() -> Unit! {
    let lib = host.dl().open("./libprobe.so")?
    if lib.get[VoidFn]("zero_symbol") is Ok(_) {
        print "zero-ok"
    } else {
        print "zero-failed"
    }
    let real = lib.get[extern(C) fn() -> i32]("probe_fn")?
    print "real=${unsafe { real.call() }}"
    if lib.get[VoidFn]("no_such_symbol") is Err(e) {
        print "missing: ${e.message()}"
    }
}

run() || (e) => print "failed: ${e.message()}"
`
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"dlnull\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
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

	cSrc := filepath.Join(root, "probe.c")
	os.WriteFile(cSrc, []byte("int probe_fn(void) { return 42; }\n"+
		"asm(\".global zero_symbol\\n.set zero_symbol, 0\\n\");\n"), 0o644)
	soPath := filepath.Join(root, "libprobe.so")
	if out, err := exec.Command("cc", "-shared", "-fPIC", "-o", soPath, cSrc).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, out)
	}

	cmd := exec.Command(exe)
	cmd.Dir = root
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatal(err, out.String())
	}
	want := "zero-ok\nreal=42\nmissing: ./libprobe.so: undefined symbol: no_such_symbol\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}
