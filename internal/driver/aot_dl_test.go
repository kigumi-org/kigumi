package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// TestAotDL loads libm with std/dl, calls two symbols through their typed
// Symbol values and sees a missing symbol as an error.
func TestAotDL(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	if _, err := os.Stat("/lib/x86_64-linux-gnu/libm.so.6"); err != nil {
		t.Skip("no libm.so.6")
	}
	root := t.TempDir()
	src := `import dl from std/dl

type Sqrt = extern(C) fn(f64) -> f64

fn run() -> Unit! {
    let lib = host.dl().open("libm.so.6")?
    let sq = lib.get[Sqrt]("sqrt")?
    let fabs: dl.Symbol[extern(C) fn(f64) -> f64] = lib.get("fabs")?
    let fl = lib.get[extern(C) fn(f64) -> f64]("floor")?
    print "${unsafe { sq.call(16.0) }} ${unsafe { fabs.call(-2.5) }} ${unsafe { fl.call(2.7) }}"
    if lib.get[Sqrt]("no_such_symbol") is Err(e) {
        print "missing"
    }
}

run() || (e) => print "failed: ${e.message()}"
`
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"dltest\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
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
		t.Fatal(err, string(out))
	}
	if string(out) != "4 2.5 2\nmissing\n" {
		t.Errorf("got %q", out)
	}
}

// TestDLFreestanding rejects std/dl on a target without a dynamic loader.
func TestDLFreestanding(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"bare\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("import dl from std/dl\n\nfn f(loader: dl.Loader) -> Unit {\n    _ = loader.open(\"x\")\n}\n"), 0o644)
	target, _ := driver.ParseTarget("x86_64-freestanding", "")
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasErrors() || !strings.Contains(res.Render(), "`std/dl` is not available on a freestanding target") {
		t.Fatalf("expected the freestanding diagnostic:\n%s", res.Render())
	}
}
