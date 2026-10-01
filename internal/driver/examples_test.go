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

// TestExampleModules checks every module under examples/ and, when the
// example ships an expect.txt, runs it in the interpreter and as a native
// executable (args.txt and stdin.txt feed both) and compares stdout. An
// aot_only marker skips the interpreter (FFI). Test blocks of the example
// must pass as well.
func TestExampleModules(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	dirs, _ := filepath.Glob("../../examples/*/mod.kg")
	if len(dirs) == 0 {
		t.Fatal("no examples found")
	}
	for _, mod := range dirs {
		root := filepath.Dir(mod)
		if _, err := os.Stat(filepath.Join(root, driver.BuildFileName)); err == nil {
			continue // driven by its build.kg: TestBuildProgramExample
		}
		t.Run(filepath.Base(root), func(t *testing.T) { runExample(t, root, std) })
	}
}

func runExample(t *testing.T, root, std string) {
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Test: true})
	if err != nil {
		t.Fatal(err)
	}
	if d := m.Diagnostics(); d != "" {
		t.Fatalf("parse diagnostics:\n%s", d)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.HasErrors() {
		t.Fatalf("check errors:\n%s", res.Render())
	}
	if hasTests(m) {
		var out, errOut bytes.Buffer
		if code, err := driver.RunTests(m, &out, &errOut); err != nil || code != 0 {
			t.Fatalf("tests: %v exit %d\n%s%s", err, code, out.String(), errOut.String())
		}
	}
	want, err := os.ReadFile(filepath.Join(root, "expect.txt"))
	if err != nil {
		return
	}
	args := []string{"main"}
	if b, err := os.ReadFile(filepath.Join(root, "args.txt")); err == nil {
		args = append(args, strings.Fields(string(b))...)
	}
	stdin, _ := os.ReadFile(filepath.Join(root, "stdin.txt"))

	plain, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	if _, aotOnly := os.Stat(filepath.Join(root, "aot_only")); aotOnly != nil {
		var out, errOut bytes.Buffer
		cwd, _ := os.Getwd()
		os.Chdir(t.TempDir())
		code, err := driver.RunInput(plain, bytes.NewReader(stdin), &out, &errOut, args)
		os.Chdir(cwd)
		if err != nil || code != 0 || out.String() != string(want) {
			t.Errorf("interp: err %v exit %d\n--- got ---\n%s--- want ---\n%s--- stderr ---\n%s", err, code, out.String(), want, errOut.String())
		}
	}

	if driver.CCompiler() == nil {
		t.Skip("no C compiler: the AOT half of this example needs zig or clang")
	}
	exe := filepath.Join(t.TempDir(), "prog")
	var buildErr bytes.Buffer
	if ok, err := testBuild(plain, exe, &buildErr); err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}
	cmd := exec.Command(exe, args[1:]...)
	cmd.Dir = t.TempDir()
	cmd.Stdin = bytes.NewReader(stdin)
	var aotOut, aotErr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &aotOut, &aotErr
	if err := cmd.Run(); err != nil || aotOut.String() != string(want) {
		t.Errorf("aot: %v\n--- got ---\n%s--- want ---\n%s--- stderr ---\n%s", err, aotOut.String(), want, aotErr.String())
	}
}

func hasTests(m *driver.Module) bool {
	for _, p := range m.Packages {
		for _, f := range p.Files {
			if strings.HasSuffix(f.File.Name, "_test.kg") {
				return true
			}
		}
	}
	return false
}
