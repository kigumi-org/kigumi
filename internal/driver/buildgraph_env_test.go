package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// A tool sees only the allowlisted environment and the target, not the
// driver's own variables.
func TestToolEnvAllowlist(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	t.Setenv("HOME", "/leaked/home")
	t.Setenv("KIGUMI_SECRET_x", "/leaked/secret")
	root, out := t.TempDir(), t.TempDir()
	write := func(rel, body string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755)
		os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644)
	}
	write("mod.kg", "Module {\n    name: \"te\"\n    kigumi: \"0.1\"\n}\n")
	write("kigumi.framework.kg", "Framework { name: \"t/env\", version: \"1\" }\nTool { name: \"env\", program: \"tools/env.sh\" }\n")
	write("tools/env.sh", "#!/bin/sh\nprintf 'HOME=%s SECRET=%s TRIPLE=%s' \"$HOME\" \"$KIGUMI_SECRET_x\" \"$KIGUMI_TARGET_TRIPLE\" > \"$1\"\n")
	os.Chmod(filepath.Join(root, "tools", "env.sh"), 0o755)
	write("cmd/one/main.kg", "print \"one\"\n")
	program := "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n" +
		"    let fw = b.framework(\"t/env\", \"1\")\n" +
		"    let o = b.runToolOutput(fw, \"env\", Array.of(\"$out:0\"), Array.empty[build.FilePath](), \"env.txt\", Array.empty[build.Secret]())?\n" +
		"    let a = b.executable(\"one\", \"cmd/one\", b.request().target())?\n    a.addSource(o)\n}\n"
	if err := buildWith(t, root, out, program); err != nil && !strings.Contains(err.Error(), "artifact") {
		t.Fatalf("build: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(out, "env.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "HOME= SECRET= TRIPLE="+driver.HostTarget().Triple {
		t.Errorf("tool environment: %q", got)
	}
}
