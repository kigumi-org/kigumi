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

// TestBuildProgramToolchain covers request 7(c): Builder.toolchain exposes
// the driver's own C compiler as tool "cc", runnable through runTool like
// any Framework-registered tool.
func TestBuildProgramToolchain(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root, out := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"tc\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "hello.c"), []byte("#include <stdio.h>\nint main(void) { printf(\"cc ok\\n\"); return 0; }\n"), 0o644)
	program := "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n" +
		"    let cc = b.toolchain()\n" +
		"    let src = b.file(\"hello.c\")?\n" +
		"    let o = b.runTool(cc, \"cc\", Array.of(\"$in:0\", \"-o\", \"$out:0\"), Array.of(src), Array.of(\"hello\"), Array.empty[build.Secret]())?\n" +
		"}\n"
	os.WriteFile(filepath.Join(root, "build.kg"), []byte(program), 0o644)
	std, _ := filepath.Abs("../../std")
	p, _, err := driver.LoadBuildProgram(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	g, err := p.Run(driver.HostTarget(), nil, out)
	if err != nil {
		t.Fatal(err)
	}
	// No kigumi.framework.kg anywhere: "toolchain" resolves without one.
	rg, err := driver.ResolveGraph(g, nil)
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if err := driver.ExecuteGraph(p, rg, out, driver.ExecOptions{}, &stderr); err != nil {
		t.Fatalf("execute: %v\n%s", err, stderr.String())
	}
	got, err := exec.Command(filepath.Join(out, "hello")).Output()
	if err != nil || string(got) != "cc ok\n" {
		t.Fatalf("hello: %q %v", got, err)
	}
}

// TestToolchainUnregisteredTool rejects a tool "toolchain" does not expose.
func TestToolchainUnregisteredTool(t *testing.T) {
	t.Parallel()
	root, out := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"tc2\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	program := "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n" +
		"    let cc = b.toolchain()\n" +
		"    let o = b.runTool(cc, \"nope\", Array.empty[String](), Array.empty[build.FilePath](), Array.empty[String](), Array.empty[build.Secret]())?\n" +
		"}\n"
	os.WriteFile(filepath.Join(root, "build.kg"), []byte(program), 0o644)
	std, _ := filepath.Abs("../../std")
	p, _, err := driver.LoadBuildProgram(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	g, err := p.Run(driver.HostTarget(), nil, out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := driver.ResolveGraph(g, nil); err == nil || !strings.Contains(err.Error(), "not registered by framework toolchain") {
		t.Fatalf("expected an unregistered-tool error, got %v", err)
	}
}
