package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kigumi/internal/driver"
)

// buildCstringNulProbe compiles src natively and returns the executable
// path; skips when no C compiler is available, matching the rest of this
// package's AOT-only tests.
func buildCstringNulProbe(t *testing.T, src string) string {
	t.Helper()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"cstringnul\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
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
	return exe
}

func runCstringNulProbe(t *testing.T, exe string) string {
	t.Helper()
	cmd := exec.Command(exe)
	cmd.Dir = t.TempDir()
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	_ = cmd.Run()
	return out.String()
}

// TestCstringNulGrepPattern covers the one embedded-NUL call site that
// TestRunCorpusEngines can't: a grep pattern with a NUL rejects on native
// (ffi.CString.new guards it), but vm/interp compile the pattern with Go's
// regexp, which has no C string to truncate and so has nothing to reject
// (finding injection-validation-1).
func TestCstringNulGrepPattern(t *testing.T) {
	const src = `import shell from std/shell
import coreutil from std/shell/coreutil

let mid = "a".toBytes().concat(Bytes.zeros(1)).concat("b".toBytes())
let pattern = String.fromBytes(mid) || "fallback"
let plan = $"printf x" |> coreutil.grep(pattern)
match shell.run(plan, host.shell()) {
    Ok(_) -> print "succeeded"
    Err(e) -> print "rejected: ${e.message()}"
}
`
	exe := buildCstringNulProbe(t, src)
	got := runCstringNulProbe(t, exe)
	want := "rejected: invalid grep pattern\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestCstringNulDlopenPath covers injection-validation-1's P1 repro: a dl
// path with an embedded NUL must be rejected before dlopen runs, not
// truncated into a shorter path that could open a real library on disk.
func TestCstringNulDlopenPath(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("no cc")
	}
	const src = `import dl from std/dl

let mid = "./libprobe.so".toBytes().concat(Bytes.zeros(1)).concat("/does/not/exist.so".toBytes())
let path = String.fromBytes(mid) || "fallback"
match host.dl().open(path) {
    Ok(_) -> print "dlopen-succeeded"
    Err(e) -> print "dlopen-failed: ${e.message()}"
}
`
	exe := buildCstringNulProbe(t, src)
	dir := filepath.Dir(exe)
	cSrc := filepath.Join(dir, "probe.c")
	os.WriteFile(cSrc, []byte("int probe_fn(void) { return 42; }\n"), 0o644)
	soPath := filepath.Join(dir, "libprobe.so")
	if out, err := exec.Command("cc", "-shared", "-fPIC", "-o", soPath, cSrc).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, out)
	}
	cmd := exec.Command(exe)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	_ = cmd.Run()
	got := out.String()
	want := "dlopen-failed: text contains a NUL byte\n"
	if got != want {
		t.Errorf("got %q, want %q (a truncated ./libprobe.so must not silently open the real library on disk)", got, want)
	}
}
