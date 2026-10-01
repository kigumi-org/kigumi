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

// TestReproducibleFreestandingArchive covers the
// freestanding archive path: BuildOptions.Reproducible builds twice under
// its own fresh build dirs and compiler caches and still produces the
// archive at out.
func TestReproducibleFreestandingArchive(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"repro\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("print \"hi\"\n"), 0o644)
	target, err := driver.ParseTarget("x86_64-freestanding", "")
	if err != nil {
		t.Fatal(err)
	}
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "repro.a")
	var stderr bytes.Buffer
	ok, err := driver.BuildWith(m, out, &stderr, driver.BuildOptions{Target: target, Reproducible: true, Env: os.Environ()})
	if err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("archive is empty")
	}
}

// TestReproducibleHostedExecutable covers a hosted
// executable: the two-build check must pass, and the resulting binary must
// still run correctly.
func TestReproducibleHostedExecutable(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"repro\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("print \"hello from repro\"\n"), 0o644)
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "prog")
	var stderr bytes.Buffer
	ok, err := driver.BuildWith(m, exe, &stderr, driver.BuildOptions{Reproducible: true, Env: os.Environ()})
	if err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, stderr.String())
	}
	cmd := exec.Command(exe)
	cmd.Dir = t.TempDir()
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\n%s", err, stderr.String())
	}
	if got, want := out.String(), "hello from repro\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

// TestReproducibleFailureLeavesNoOutput forces the linker to embed a fresh
// random build ID (`--build-id=uuid`) on every invocation, so the two
// --reproducible builds are guaranteed to genuinely differ regardless of
// timing, and checks that BuildWith reports the mismatch and never writes
// to out.
func TestReproducibleFailureLeavesNoOutput(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"repro\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("print \"hi\"\n"), 0o644)
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "prog")
	var stderr bytes.Buffer
	ok, err := driver.BuildWith(m, out, &stderr, driver.BuildOptions{Reproducible: true, Env: os.Environ(), LinkFlags: []string{"-Wl,--build-id=uuid"}})
	if ok || err == nil {
		t.Fatalf("expected a reproducibility failure, got ok=%v err=%v", ok, err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "is not reproducible") || !strings.Contains(msg, "byte offset") {
		t.Errorf("diagnostic %q does not name the file and a byte offset", msg)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Errorf("out should not exist after a reproducibility failure, stat: %v", statErr)
	}
}

// TestReproducibleOutputSymlinkEscape covers BUILD-002 for
// buildReproducible's own final write: a symlink pre-planted at out must
// not be followed to write the compiled artifact outside the sandbox.
func TestReproducibleOutputSymlinkEscape(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"repro\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("print \"hi\"\n"), 0o644)
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	outDir, outside := t.TempDir(), t.TempDir()
	victim := filepath.Join(outside, "victim.txt")
	os.WriteFile(victim, []byte("ORIGINAL"), 0o644)
	out := filepath.Join(outDir, "prog")
	if err := os.Symlink(victim, out); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	ok, err := driver.BuildWith(m, out, &stderr, driver.BuildOptions{Reproducible: true, Env: os.Environ()})
	if ok || err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected a symlink refusal, got ok=%v err=%v", ok, err)
	}
	got, readErr := os.ReadFile(victim)
	if readErr != nil || string(got) != "ORIGINAL" {
		t.Fatalf("victim file must be untouched: %q %v", got, readErr)
	}
}

// chdir changes the working directory to dir and restores it on cleanup;
// used to exercise a relative module root the way `kigumi build --reproducible .`
// passes one.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

// TestReproducibleHostedSourceRelativeRoot covers a hosted module with a
// native `Source` file, loaded from a relative root (the shape of `kigumi
// build --reproducible -o prog .`): the hosted-link pinCompDir branch must
// not crash trying to make m.Sources' relative path relative to the build
// dir's own absolute os.MkdirTemp path.
func TestReproducibleHostedSourceRelativeRoot(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"repro\"\n    kigumi: \"0.1\"\n}\nSource { c: \"native.c\" }\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("print \"hi from native\"\n"), 0o644)
	os.WriteFile(filepath.Join(root, "native.c"), []byte("int kigumi_repro_native(void) { return 0; }\n"), 0o644)
	chdir(t, root)
	m, err := driver.LoadModule(".", driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "prog")
	var stderr bytes.Buffer
	ok, err := driver.BuildWith(m, exe, &stderr, driver.BuildOptions{Reproducible: true, Env: os.Environ()})
	if err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, stderr.String())
	}
}

// TestReproducibleFreestandingSourceRelativeRoot is
// TestReproducibleHostedSourceRelativeRoot for the freestanding archive
// path, which resolves inputs relative to its build dir unconditionally
// (buildArchive) and has the same crash for a
// relative module root.
func TestReproducibleFreestandingSourceRelativeRoot(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"repro\"\n    kigumi: \"0.1\"\n}\nSource { c: \"native.c\" }\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("print \"hi from native\"\n"), 0o644)
	os.WriteFile(filepath.Join(root, "native.c"), []byte("int kigumi_repro_native(void) { return 0; }\n"), 0o644)
	target, err := driver.ParseTarget("x86_64-freestanding", "")
	if err != nil {
		t.Fatal(err)
	}
	chdir(t, root)
	m, err := driver.LoadModule(".", driver.LoadOptions{StdRoot: std, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "repro.a")
	var stderr bytes.Buffer
	ok, err := driver.BuildWith(m, out, &stderr, driver.BuildOptions{Target: target, Reproducible: true, Env: os.Environ()})
	if err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, stderr.String())
	}
	if data, err := os.ReadFile(out); err != nil || len(data) == 0 {
		t.Fatalf("archive missing or empty: %v", err)
	}
}

// TestReproducibleHostedLinkSearchRelativeRoot checks a relative
// `Link { search: ... }` path (addNative, mirroring the Source case) resolves
// against the process cwd, not the hosted-link pinCompDir build dir that
// cmd.Dir points at.
func TestReproducibleHostedLinkSearchRelativeRoot(t *testing.T) {
	cc := driver.CCompiler()
	if cc == nil {
		t.Skip("no C compiler")
	}
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"repro\"\n    kigumi: \"0.1\"\n}\nLink { library: \"reprofoo\", search: \"libs\" }\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("print \"hi\"\n"), 0o644)
	libs := filepath.Join(root, "libs")
	os.Mkdir(libs, 0o755)
	os.WriteFile(filepath.Join(libs, "foo.c"), []byte("int kigumi_repro_foo(void) { return 0; }\n"), 0o644)
	soArgs := append(append([]string{}, cc[1:]...), "-shared", "-fPIC", "-o", filepath.Join(libs, "libreprofoo.so"), filepath.Join(libs, "foo.c"))
	if out, err := exec.Command(cc[0], soArgs...).CombinedOutput(); err != nil {
		t.Fatalf("build libreprofoo.so: %v\n%s", err, out)
	}
	chdir(t, root)
	m, err := driver.LoadModule(".", driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "prog")
	var stderr bytes.Buffer
	ok, err := driver.BuildWith(m, exe, &stderr, driver.BuildOptions{Reproducible: true, Env: os.Environ()})
	if err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, stderr.String())
	}
}
