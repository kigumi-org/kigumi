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

// buildWith writes build.kg into root and runs the whole pipeline; the
// returned error is the first failure along load, run, resolve, execute.
func buildWith(t *testing.T, root, out, program string) error {
	t.Helper()
	os.WriteFile(filepath.Join(root, "build.kg"), []byte(program), 0o644)
	std, _ := filepath.Abs("../../std")
	p, _, err := driver.LoadBuildProgram(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		return err
	}
	g, err := p.Run(driver.HostTarget(), nil, out)
	if err != nil {
		return err
	}
	roots, _ := p.DescriptorRoots()
	descriptors, err := driver.LoadFrameworks(roots)
	if err != nil {
		return err
	}
	rg, err := driver.ResolveGraph(g, descriptors)
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	return driver.ExecuteGraph(p, rg, out, driver.ExecOptions{ToolEnv: []string{"PATH=" + os.Getenv("PATH")}}, &stderr)
}

// TestBuildProgramSandbox covers the review findings: symlink and link
// escapes, descriptors planted in the output directory, malformed
// placeholders, duplicate configure, own-package imports, and distinct
// entries for several executables.
func TestBuildProgramSandbox(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root, out, outside := t.TempDir(), t.TempDir(), t.TempDir()
	write := func(rel, body string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755)
		os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644)
	}
	write("mod.kg", "Module {\n    name: \"sb\"\n    kigumi: \"0.1\"\n}\n")
	write("kigumi.framework.kg", "Framework { name: \"t/cp\", version: \"1\" }\nTool { name: \"cp\", program: \"tools/cp.sh\" }\n")
	write("tools/cp.sh", "#!/bin/sh\ncp \"$1\" \"$2\"\n")
	os.Chmod(filepath.Join(root, "tools", "cp.sh"), 0o755)
	write("cmd/one/main.kg", "print \"one\"\n")
	write("cmd/two/main.kg", "print \"two\"\n")
	write("greet/lib.kg", "pub fn hi() -> Int {\n    1\n}\n")
	os.WriteFile(filepath.Join(outside, "secret.c"), []byte("int leak(void) { return 1; }\n"), 0o644)
	os.Symlink(outside, filepath.Join(root, "escape"))
	head := "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n"
	host := "b.request().target()"

	err := buildWith(t, root, out, head+"    let f = b.file(\"escape/secret.c\")?\n    let a = b.executable(\"one\", \"cmd/one\", "+host+")?\n    a.addSource(f)\n}\n")
	if err == nil || !strings.Contains(err.Error(), "outside") {
		t.Errorf("symlink escape: %v", err)
	}
	err = buildWith(t, root, out, head+"    let a = b.executable(\"one\", \"cmd/one\", "+host+")?\n    a.addLink(\"m\", \""+outside+"\")\n}\n")
	if err == nil || !strings.Contains(err.Error(), "outside") {
		t.Errorf("link search escape: %v", err)
	}
	os.WriteFile(filepath.Join(out, "kigumi.framework.kg"), []byte("Framework { name: \"evil/x\", version: \"1\" }\nTool { name: \"pwn\", program: \"/bin/true\" }\n"), 0o644)
	err = buildWith(t, root, out, head+"    let fw = b.framework(\"evil/x\", \"1\")\n    let o = b.runToolOutput(fw, \"pwn\", Array.of(\"$out:0\"), Array.empty[build.FilePath](), \"x.txt\", Array.empty[build.Secret]())?\n    let a = b.executable(\"one\", \"cmd/one\", "+host+")?\n}\n")
	if err == nil || !strings.Contains(err.Error(), "no descriptor") {
		t.Errorf("descriptor in the output directory must not count: %v", err)
	}
	err = buildWith(t, root, out, head+"    let fw = b.framework(\"t/cp\", \"1\")\n    let src = b.file(\"tools/cp.sh\")?\n    let o = b.runToolOutput(fw, \"cp\", Array.of(\"$in:0garbage\", \"$out:0\"), Array.of(src), \"copy.sh\", Array.empty[build.Secret]())?\n    let a = b.executable(\"one\", \"cmd/one\", "+host+")?\n}\n")
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Errorf("malformed placeholder: %v", err)
	}
	err = buildWith(t, root, out, "import build from std/build\n\nfn configure(x: Int) -> Unit! {\n    let y = x\n}\n\nfn configure(b: build.Builder) -> Unit! {\n    let a = b.executable(\"one\", \"cmd/one\", "+host+")?\n}\n")
	if err == nil || !strings.Contains(err.Error(), "declares configure 2 times") {
		t.Errorf("overloaded configure: %v", err)
	}
	err = buildWith(t, root, out, "import build from std/build\nimport greet from sb/greet\n\nfn configure(b: build.Builder) -> Unit! {\n    let a = b.executable(\"one\", \"cmd/one\", "+host+")?\n}\n")
	if err == nil || !strings.Contains(err.Error(), "cannot import") {
		t.Errorf("own package import: %v", err)
	}
	if err := buildWith(t, root, out, head+"    let a = b.executable(\"one\", \"cmd/one\", "+host+")?\n    let c = b.executable(\"two\", \"cmd/two\", "+host+")?\n}\n"); err != nil {
		t.Fatalf("two executables: %v", err)
	}
	for _, name := range []string{"one", "two"} {
		got, err := exec.Command(filepath.Join(out, name)).Output()
		if err != nil || string(got) != name+"\n" {
			t.Errorf("%s printed %q (%v)", name, got, err)
		}
	}
}
