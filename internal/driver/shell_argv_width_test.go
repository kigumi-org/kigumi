package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

const shellArgvWidthSrc = `import shell from std/shell

let sh = host.shell()
let status = ($"printf %s-%s-%s a b c" |> shell.run(sh))?
shell.check(status)?
`

// funcBody returns the LLVM IR of the `define ... @name(` function, up to
// the next top-level `define`.
func funcBody(t *testing.T, ir, name string) string {
	t.Helper()
	at := strings.Index(ir, "@\""+name+"\"(")
	if at < 0 {
		at = strings.Index(ir, "@"+name+"(")
	}
	if at < 0 {
		t.Fatalf("function %q not found in IR", name)
	}
	start := strings.LastIndex(ir[:at], "\ndefine ")
	if start < 0 {
		t.Fatalf("no `define` line before %q", name)
	}
	rest := ir[start+1:]
	end := strings.Index(rest, "\ndefine ")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

// TestShellArgvUsesTargetPointerWidth covers pointer-width-layout-5:
// runChild must size the execvp() argv array off the target's real pointer
// width (4 bytes on a 32-bit target), not a hardcoded 8-byte stride.
func TestShellArgvUsesTargetPointerWidth(t *testing.T) {
	target, err := driver.ParseTarget("x86-linux-musl", "")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"argvwidth\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte(shellArgvWidthSrc), 0o644)
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	ir, ok, err := driver.Compile(m, &stderr)
	if err != nil || !ok {
		t.Fatalf("compile: %v\n%s", err, stderr.String())
	}
	if !strings.Contains(ir, "@malloc(i32") {
		t.Fatalf("expected the 32-bit target's malloc to take i32, got:\n%s", grepLines(ir, "declare ptr @malloc"))
	}
	body := funcBody(t, ir, "std/shell.runChild")
	if !strings.Contains(body, "rt_int(i64 4, i32 32)") {
		t.Errorf("runChild should size the argv array off the target's 4-byte pointer width; got:\n%s", grepLines(body, "rt_int(i64 "))
	}
	if strings.Contains(body, "rt_int(i64 8, i32 32)") {
		t.Errorf("runChild still hardcodes an 8-byte pointer stride on a 32-bit target:\n%s", grepLines(body, "rt_int(i64 8"))
	}
}
