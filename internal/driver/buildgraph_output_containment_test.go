package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildOutputSymlinkEscape covers BUILD-002: a symlink pre-planted at
// a Builder.write output name must not be followed to write outside the
// output directory.
func TestBuildOutputSymlinkEscape(t *testing.T) {
	t.Parallel()
	root, out, outside := t.TempDir(), t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"oc\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	victim := filepath.Join(outside, "victim.txt")
	os.WriteFile(victim, []byte("ORIGINAL"), 0o644)
	os.Symlink(victim, filepath.Join(out, "gen.txt"))
	program := "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n" +
		"    let f = b.write(\"gen.txt\", \"PWNED\")?\n" +
		"}\n"
	err := buildWith(t, root, out, program)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected a symlink refusal, got %v", err)
	}
	got, readErr := os.ReadFile(victim)
	if readErr != nil || string(got) != "ORIGINAL" {
		t.Fatalf("victim file must be untouched: %q %v", got, readErr)
	}
}

// TestBuildOutputDotDotName covers BUILD-001: an output name of ".." must
// be rejected before it can be joined into a path outside the output
// directory, not merely fail with an unrelated filesystem error.
func TestBuildOutputDotDotName(t *testing.T) {
	t.Parallel()
	root, out := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"oc2\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	program := "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n" +
		"    let f = b.write(\"..\", \"PWNED\")?\n" +
		"}\n"
	err := buildWith(t, root, out, program)
	if err == nil || !strings.Contains(err.Error(), "bare file name") {
		t.Fatalf("expected a bare-name refusal at declaration time, got %v", err)
	}
}

// TestBuildToolOutputDirectory covers BUILD-004: a tool step that leaves a
// directory at its declared output must fail the build, not proceed with
// a directory masquerading as a produced file.
func TestBuildToolOutputDirectory(t *testing.T) {
	t.Parallel()
	root, out := t.TempDir(), t.TempDir()
	write := func(rel, body string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755)
		os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644)
	}
	write("mod.kg", "Module {\n    name: \"oc3\"\n    kigumi: \"0.1\"\n}\n")
	write("kigumi.framework.kg", "Framework { name: \"t/mkdir\", version: \"1\" }\nTool { name: \"mkdir\", program: \"tools/mkdir.sh\" }\n")
	write("tools/mkdir.sh", "#!/bin/sh\nmkdir -p \"$1\"\n")
	os.Chmod(filepath.Join(root, "tools", "mkdir.sh"), 0o755)
	program := "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n" +
		"    let fw = b.framework(\"t/mkdir\", \"1\")\n" +
		"    let o = b.runToolOutput(fw, \"mkdir\", Array.of(\"$out:0\"), Array.empty[build.FilePath](), \"gen.c\", Array.empty[build.Secret]())?\n" +
		"}\n"
	err := buildWith(t, root, out, program)
	if err == nil || !strings.Contains(err.Error(), "did not produce a regular file") {
		t.Fatalf("expected a not-a-regular-file refusal, got %v", err)
	}
}

// TestBuildOutputReservedDeviceName rejects the Windows reserved device
// names (CON/PRN/AUX/NUL/COMn/LPTn) as declared output and artifact names,
// case-insensitively and regardless of any extension, on every host: a
// build.kg from a fetched dependency should not produce a graph that only
// breaks once someone runs it on Windows.
func TestBuildOutputReservedDeviceName(t *testing.T) {
	root, out := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"oc4\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	for _, name := range []string{"nul", "NUL", "Con.txt", "com1", "lpt9"} {
		program := "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n" +
			"    let f = b.write(\"" + name + "\", \"PWNED\")?\n" +
			"}\n"
		if err := buildWith(t, root, out, program); err == nil || !strings.Contains(err.Error(), "bare file name") {
			t.Fatalf("write(%q): expected a bare-name refusal, got %v", name, err)
		}
	}
	program := "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n" +
		"    let a = b.executable(\"con\", \"cmd/con\", b.request().target())?\n" +
		"}\n"
	if err := buildWith(t, root, out, program); err == nil || !strings.Contains(err.Error(), "bare name") {
		t.Fatalf("executable(\"con\"): expected a bare-name refusal, got %v", err)
	}

	// CreateFile reserves these names at any path component, so a
	// directory segment named "nul" is just as broken on Windows as an
	// artifact or output leaf name.
	program = "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n" +
		"    let a = b.executable(\"app\", \"cmd/nul/inner\", b.request().target())?\n" +
		"}\n"
	if err := buildWith(t, root, out, program); err == nil || !strings.Contains(err.Error(), "root-relative directory") {
		t.Fatalf("executable dir with a reserved segment: expected a directory refusal, got %v", err)
	}
}

// TestBuildGraphGeneratedOutputIgnoresStaleMode covers a regression from
// file-writes-safety-1: writeFileAtomic's mode-preserving behavior (meant
// for user source edited by `kigumi fmt -w`/`kigumi get`) must not leak
// into build-graph "generated" files. A rebuild is a deterministic
// function of the graph, so a slot a previous run (or a user) left
// chmod'd must not survive into the freshly generated content.
func TestBuildGraphGeneratedOutputIgnoresStaleMode(t *testing.T) {
	root, out := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"oc5\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	program := "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n" +
		"    let f = b.write(\"gen.txt\", \"hello\")?\n" +
		"}\n"
	if err := buildWith(t, root, out, program); err != nil {
		t.Fatalf("first build: %v", err)
	}
	genPath := filepath.Join(out, "gen.txt")
	if err := os.Chmod(genPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := buildWith(t, root, out, program); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	fi, err := os.Stat(genPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o644 {
		t.Fatalf("rebuild must reset gen.txt to the declared mode 0644, got %#o (stale mode leaked through)", got)
	}
}
