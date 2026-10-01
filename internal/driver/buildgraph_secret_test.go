package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// A failing tool step must never let the secret's bytes, or the ephemeral
// temp file path holding them, reach the error or the tool's own log.
func TestToolStepFailureRedactsSecret(t *testing.T) {
	t.Parallel()
	root, out := t.TempDir(), t.TempDir()
	write := func(rel, body string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755)
		os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644)
	}
	write("mod.kg", "Module {\n    name: \"se\"\n    kigumi: \"0.1\"\n}\n")
	write("kigumi.framework.kg", "Framework { name: \"t/leak\", version: \"1\" }\nTool { name: \"leak\", program: \"tools/leak.sh\" }\n")
	write("tools/leak.sh", "#!/bin/sh\nexit 1\n")
	os.Chmod(filepath.Join(root, "tools", "leak.sh"), 0o755)
	write("build.kg", "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n"+
		"    let fw = b.framework(\"t/leak\", \"1\")\n"+
		"    let sec = b.secret(\"apikey\")\n"+
		"    let o = b.runToolOutput(fw, \"leak\", Array.of(\"$secret:0\"), Array.empty[build.FilePath](), \"out.txt\", Array.of(sec))?\n"+
		"}\n")
	secretFile := filepath.Join(root, "apikey.secret")
	os.WriteFile(secretFile, []byte("super-secret-value"), 0o600)

	std, _ := filepath.Abs("../../std")
	p, _, err := driver.LoadBuildProgram(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	g, err := p.Run(driver.HostTarget(), nil, out)
	if err != nil {
		t.Fatal(err)
	}
	roots, _ := p.DescriptorRoots()
	descriptors, err := driver.LoadFrameworks(roots)
	if err != nil {
		t.Fatal(err)
	}
	rg, err := driver.ResolveGraph(g, descriptors)
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	execErr := driver.ExecuteGraph(p, rg, out, driver.ExecOptions{
		Secrets: map[string]string{"apikey": secretFile},
		ToolEnv: []string{"PATH=" + os.Getenv("PATH")},
	}, &stderr)
	if execErr == nil {
		t.Fatal("the tool exits 1, ExecuteGraph should report the failure")
	}
	for _, s := range []string{execErr.Error(), stderr.String()} {
		if strings.Contains(s, "super-secret-value") {
			t.Fatalf("secret bytes leaked: %q", s)
		}
		if strings.Contains(s, "kigumi-secret-") {
			t.Fatalf("secret temp file path leaked: %q", s)
		}
	}
	if !strings.Contains(execErr.Error(), "$secret:apikey") {
		t.Fatalf("error should name the redacted placeholder, got: %v", execErr)
	}
}

// A generated file (Builder.write) is published through a temp file
// renamed over the target: a directory the driver cannot write into fails
// the build cleanly, with no partial file and no leftover temp file.
func TestGeneratedFileWriteFailsCleanlyOnReadOnlyDir(t *testing.T) {
	t.Parallel()
	root, out := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"ro\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "build.kg"), []byte("import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n"+
		"    let o = b.write(\"gen.txt\", \"hello from generated file\\n\")?\n"+
		"}\n"), 0o644)

	std, _ := filepath.Abs("../../std")
	p, _, err := driver.LoadBuildProgram(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	g, err := p.Run(driver.HostTarget(), nil, out)
	if err != nil {
		t.Fatal(err)
	}
	rg, err := driver.ResolveGraph(g, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(out, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(out, 0o755)

	var stderr bytes.Buffer
	execErr := driver.ExecuteGraph(p, rg, out, driver.ExecOptions{}, &stderr)
	if execErr == nil {
		t.Fatal("a read-only output directory should fail the build, not silently succeed")
	}
	if _, statErr := os.Stat(filepath.Join(out, "gen.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("no partial file should exist at the target path, stat: %v", statErr)
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("no temp file should be left in the output directory, found: %v", entries)
	}
}

// When a generated file's target already exists, a write that fails before
// it can rename over the target must leave the old content untouched.
func TestGeneratedFileWriteAtomicKeepsOldContentOnFailure(t *testing.T) {
	t.Parallel()
	root, out := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"ro2\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "build.kg"), []byte("import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n"+
		"    let o = b.write(\"gen.txt\", \"new content\\n\")?\n"+
		"}\n"), 0o644)
	os.WriteFile(filepath.Join(out, "gen.txt"), []byte("original content\n"), 0o644)

	std, _ := filepath.Abs("../../std")
	p, _, err := driver.LoadBuildProgram(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	g, err := p.Run(driver.HostTarget(), nil, out)
	if err != nil {
		t.Fatal(err)
	}
	rg, err := driver.ResolveGraph(g, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(out, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(out, 0o755)

	var stderr bytes.Buffer
	if err := driver.ExecuteGraph(p, rg, out, driver.ExecOptions{}, &stderr); err == nil {
		t.Fatal("a read-only output directory should fail the build, not silently succeed")
	}
	got, err := os.ReadFile(filepath.Join(out, "gen.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original content\n" {
		t.Fatalf("old content should survive a failed write untouched, got %q", got)
	}
}
