package cli_test

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/cli"
	"kigumi/internal/cliutil"
)

func execute(t *testing.T, args ...string) (int, string, string) {
	root := cli.RootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	code := cliutil.Execute(root, cli.Debug(root))
	return code, out.String(), errOut.String()
}

// executeReal is like execute, but also returns what cliutil.Execute
// wrote directly to os.Stderr (usage and top-level errors bypass the
// command's own SetErr writer).
func executeReal(t *testing.T, args ...string) (int, string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	real := os.Stderr
	os.Stderr = w
	code, _, errOut := execute(t, args...)
	os.Stderr = real
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return code, errOut + buf.String()
}

func TestCommands(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	code, _, _ := execute(t, "--std", std, "check", "../../examples/repostat")
	if code != 0 {
		t.Fatalf("check repostat: exit %d", code)
	}
	if code, out, _ := execute(t, "--std", std, "../../examples/hello/main.kg"); code != 0 || out != "Hello, Kigumi!\n" {
		t.Fatalf("run root-level entry: exit %d, stdout %q", code, out)
	}
	if code, _, errOut := execute(t, "--std", std, "check", "../../examples/repostat/cmd/repostat/main.kg"); code != 0 {
		t.Fatalf("check repostat entry file: exit %d, stderr %q", code, errOut)
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "main"), 0o755)
	file := filepath.Join(dir, "main", "main.kg")
	os.WriteFile(file, []byte("let x: String = 1\nprint x\n"), 0o644)
	code, _, errOut := execute(t, "--std", std, "check", file)
	if code != 1 || !strings.Contains(errOut, "expected `String`, found `Int`") {
		t.Fatalf("check with type error: exit %d, stderr %q", code, errOut)
	}
	os.WriteFile(file, []byte("let  x = 1\nprint \"v=${x}\"\n"), 0o644)
	code, _, errOut = execute(t, "fmt", "--check", file)
	if code != 1 || !strings.Contains(errOut, "not formatted") {
		t.Fatalf("fmt --check: exit %d, stderr %q", code, errOut)
	}
	code, out, _ := execute(t, "--std", std, "run", file, "extra")
	if code != 0 || out != "v=1\n" {
		t.Fatalf("run: exit %d, stdout %q", code, out)
	}
	code, out, _ = execute(t, "--std", std, file, "--not-a-flag")
	if code != 0 || out != "v=1\n" {
		t.Fatalf("run via bare file: exit %d, stdout %q", code, out)
	}
	if code, out, _ = execute(t); code != 0 || !strings.Contains(out, "Usage:") {
		t.Fatalf("no arguments: exit %d, stdout %q", code, out)
	}
	if code, _, _ = execute(t, "bogus"); code != 2 {
		t.Fatalf("unknown command: exit %d", code)
	}
	if code, _, _ = execute(t, "check"); code != 2 {
		t.Fatalf("missing argument: exit %d", code)
	}
	if code, _, _ = execute(t, "--bogus"); code != 2 {
		t.Fatalf("unknown flag: exit %d", code)
	}
	if code, _, _ = execute(t, "check", filepath.Join(dir, "missing")); code != 1 {
		t.Fatalf("missing module: exit %d", code)
	}
}

// A nonexistent .kg entry file must fail loudly, the same way a
// nonexistent module-root directory already does, rather than loading an
// empty or unrelated module and reporting success.
func TestMissingEntryFile(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "other.kg"), []byte("fn main() -> Unit! {}\n"), 0o644)
	missing := filepath.Join(dir, "does_not_exist.kg")

	for _, sub := range []string{"check", "test", "doc"} {
		code, errOut := executeReal(t, "--std", std, sub, missing)
		if code != 1 || !strings.Contains(errOut, "no such file") {
			t.Errorf("%s on missing entry file: exit %d, stderr %q", sub, code, errOut)
		}
	}

	out := filepath.Join(dir, "a.out")
	code, errOut := executeReal(t, "--std", std, "build", "-o", out, missing)
	if code != 1 || !strings.Contains(errOut, "no such file") {
		t.Errorf("build on missing entry file: exit %d, stderr %q", code, errOut)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("build on missing entry file wrote an output binary")
	}

	code, errOut = executeReal(t, "--std", std, "run", missing)
	if code != 1 || !strings.Contains(errOut, "no such file") {
		t.Errorf("run on missing entry file: exit %d, stderr %q", code, errOut)
	}
	if strings.Contains(errOut, "top-level statements") || strings.Contains(errOut, "no entry") {
		t.Errorf("run on missing entry file leaked an unrelated diagnostic: %q", errOut)
	}
}

// `run` places [flags] before the entry file in its own --help text, and
// must actually reject one placed after: SetInterspersed(false) otherwise
// forwards it to the script with no warning.
func TestRunFlagAfterEntryFile(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	dir := t.TempDir()
	file := filepath.Join(dir, "main.kg")
	os.WriteFile(file, []byte("print \"ok\"\n"), 0o644)

	code, errOut := executeReal(t, "--std", std, "run", "--target", "bogus-triple-xyz", file)
	if code != 2 {
		t.Fatalf("--target before the entry file: exit %d, stderr %q", code, errOut)
	}
	code, errOut = executeReal(t, "--std", std, "run", file, "--target", "bogus-triple-xyz")
	if code != 2 || !strings.Contains(errOut, "--target") {
		t.Fatalf("--target after the entry file: exit %d, stderr %q", code, errOut)
	}
	code, out, _ := execute(t, "--std", std, "run", file, "extra", "positional")
	if code != 0 || out != "ok\n" {
		t.Fatalf("plain args after the entry file: exit %d, stdout %q", code, out)
	}
}

// A literal "--" after the entry file must end the flag scan, so a script
// argument that happens to look like a kigumi flag (e.g. "--target") still
// reaches the script instead of being rejected.
func TestRunDashDashForwardsScriptArgv(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	dir := t.TempDir()
	file := filepath.Join(dir, "main.kg")
	os.WriteFile(file, []byte(`let args = host.args()
for i in 1..args.len() {
    print "${args.get(i) || "?"}"
}
`), 0o644)

	code, out, errOut := execute(t, "--std", std, "run", file, "--", "--target", "literal")
	if code != 0 {
		t.Fatalf("run with -- before flag-shaped argv: exit %d, stderr %q", code, errOut)
	}
	if want := "--target\nliteral\n"; out != want {
		t.Fatalf("script argv after --: got %q, want %q", out, want)
	}
}

// A mistyped .kg script argument to `get` must be reported as a missing
// file, not silently reclassified as a module path to add.
func TestGetMissingScriptArg(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing_script.kg")
	if code, errOut := executeReal(t, "get", missing); code != 2 || !strings.Contains(errOut, "no such file") {
		t.Fatalf("get with no mod.kg: exit %d, stderr %q", code, errOut)
	}
	os.WriteFile(filepath.Join(dir, "mod.kg"), []byte("Module {\n    name: \"example\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	typo := filepath.Join(dir, "typo_script.kg")
	if code, errOut := executeReal(t, "get", typo); code != 2 || !strings.Contains(errOut, "no such file") {
		t.Fatalf("get with a mod.kg present: exit %d, stderr %q", code, errOut)
	}
}

// TestGet fetches a dependency from a local git repository tagged with the
// required version, then loads the module that needs it.
func TestGet(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "greet.git")
	os.MkdirAll(repo, 0o755)
	os.WriteFile(filepath.Join(repo, "mod.kg"), []byte("Module {\n    name: \"greet\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "greet.kg"), []byte("pub fn hello(name: String) -> String {\n    \"hello, ${name}\"\n}\n"), 0o644)
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "-m", "v0.1.0"}, {"tag", "v0.1.0"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git unavailable: %v %s", err, out)
		}
	}
	app := filepath.Join(root, "app")
	os.MkdirAll(app, 0o755)
	os.WriteFile(filepath.Join(app, "mod.kg"), []byte("Module {\n    name: \"app\"\n    kigumi: \"0.1\"\n}\nRequire { name: \"greet\", version: \"v0.1.0\", url: \""+repo+"\" }\n"), 0o644)
	os.WriteFile(filepath.Join(app, "main.kg"), []byte("import {hello} from greet\nprint hello(\"deps\")\n"), 0o644)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	std, _ := filepath.Abs("../../std")
	if code, _, _ := execute(t, "--std", std, "run", app); code == 0 {
		t.Fatal("run before get should fail")
	}
	if code, out, errOut := execute(t, "get", app); code != 0 || !strings.Contains(out, "fetched") {
		t.Fatalf("get: exit %d out %q err %q", code, out, errOut)
	}
	if code, out, errOut := execute(t, "--std", std, "run", app); code != 0 || out != "hello, deps\n" {
		t.Fatalf("run after get: exit %d out %q err %q", code, out, errOut)
	}
	if code, out, _ := execute(t, "get", app); code != 0 || strings.Contains(out, "fetched") {
		t.Fatalf("second get should reuse the cache: %q", out)
	}
}
