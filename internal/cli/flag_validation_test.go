package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An unrecognized --engine value must be a usage error, not a silent
// alias for "auto".
func TestEngineFlagValidation(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	hello, _ := filepath.Abs("../../examples/hello/main.kg")
	testing_, _ := filepath.Abs("../../examples/testing")

	if code, errOut := executeReal(t, "--std", std, "run", "--engine", "bogus", hello); code != 2 || !strings.Contains(errOut, "--engine") {
		t.Fatalf("run --engine bogus: exit %d, stderr %q", code, errOut)
	}
	if code, errOut := executeReal(t, "--std", std, "run", "--engine", "VM", hello); code != 2 || !strings.Contains(errOut, "--engine") {
		t.Fatalf("run --engine VM (case typo): exit %d, stderr %q", code, errOut)
	}
	if code, out, errOut := execute(t, "--std", std, "run", "--engine", "vm", hello); code != 0 || out != "Hello, Kigumi!\n" {
		t.Fatalf("run --engine vm: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if code, errOut := executeReal(t, "--std", std, "test", "--engine", "bogus", testing_); code != 2 || !strings.Contains(errOut, "--engine") {
		t.Fatalf("test --engine bogus: exit %d, stderr %q", code, errOut)
	}

	// An invalid --engine value must be reported even when the target
	// module itself fails to load.
	dir := t.TempDir()
	broken := filepath.Join(dir, "main.kg")
	os.WriteFile(broken, []byte("fn main() -> Unit! {\n"), 0o644)
	if code, errOut := executeReal(t, "--std", std, "run", "--engine", "bogus", broken); code != 2 || !strings.Contains(errOut, "--engine") {
		t.Fatalf("run --engine bogus on a broken module: exit %d, stderr %q", code, errOut)
	}
	if code, errOut := executeReal(t, "--std", std, "test", "--engine", "bogus", dir); code != 2 || !strings.Contains(errOut, "--engine") {
		t.Fatalf("test --engine bogus on a broken module: exit %d, stderr %q", code, errOut)
	}
}

// An unrecognized --color value must be a usage error rather than
// silently falling back to auto-detection.
func TestColorFlagValidation(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	hello, _ := filepath.Abs("../../examples/hello/main.kg")

	if code, errOut := executeReal(t, "--color", "banana", "--std", std, "run", hello); code != 2 || !strings.Contains(errOut, "--color") {
		t.Fatalf("--color banana: exit %d, stderr %q", code, errOut)
	}
	if code, out, errOut := execute(t, "--color", "never", "--std", std, "run", hello); code != 0 || out != "Hello, Kigumi!\n" {
		t.Fatalf("--color never: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

// `build --plan/--out-dir/--secret` only mean anything for a module whose
// build.kg has a Program.
func TestBuildRejectsBuildProgramFlagsWithoutOne(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	dir := t.TempDir()
	file := filepath.Join(dir, "main.kg")
	os.WriteFile(file, []byte("fn main() -> Unit! {}\n"), 0o644)
	out := filepath.Join(dir, "a.out")

	if code, errOut := executeReal(t, "--std", std, "build", "--plan", "-o", out, file); code != 2 || !strings.Contains(errOut, "--plan") {
		t.Fatalf("build --plan without build.kg: exit %d, stderr %q", code, errOut)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("build --plan without build.kg must not build anything")
	}
	if code, errOut := executeReal(t, "--std", std, "build", "--out-dir", dir, "-o", out, file); code != 2 || !strings.Contains(errOut, "--out-dir") {
		t.Fatalf("build --out-dir without build.kg: exit %d, stderr %q", code, errOut)
	}
	if code, errOut := executeReal(t, "--std", std, "build", "--secret", "x=/nonexistent", "-o", out, file); code != 2 || !strings.Contains(errOut, "--secret") {
		t.Fatalf("build --secret without build.kg: exit %d, stderr %q", code, errOut)
	}
	if code, _, _ := execute(t, "--std", std, "build", "-o", out, file); code != 0 {
		t.Fatalf("plain build without those flags should still work: exit %d", code)
	}
}

// `fmt --check -w` must be a usage error rather than silently picking
// --check.
func TestFmtCheckWriteMutuallyExclusive(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.kg")
	src := "let   x = 1\nprint x\n"
	os.WriteFile(file, []byte(src), 0o644)

	code, errOut := executeReal(t, "fmt", "--check", "-w", file)
	if code != 2 || !strings.Contains(errOut, "--check") || !strings.Contains(errOut, "--write") {
		t.Fatalf("fmt --check -w: exit %d, stderr %q", code, errOut)
	}
	got, err := os.ReadFile(file)
	if err != nil || string(got) != src {
		t.Fatalf("fmt --check -w must not touch the file: %q, %v", got, err)
	}
}
