package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `kigumi doc -o <dir>` writes must go through the same atomic path as
// `fmt -w`, so a failed write can't truncate a colliding pre-existing file.
func TestDocWritePreservesExistingFileOnFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	std, _ := filepath.Abs("../../std")
	mod := t.TempDir()
	os.WriteFile(filepath.Join(mod, "mod.kg"), []byte("Module {\n    name: \"greet\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(mod, "greet.kg"), []byte("/// Says hi.\npub fn hello() -> String {\n    \"hi\"\n}\n"), 0o644)

	outDir := t.TempDir()
	preexisting := "this is real content that must survive a failed doc write"
	readme := filepath.Join(outDir, "README.md")
	os.WriteFile(readme, []byte(preexisting), 0o644)

	if err := os.Chmod(outDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(outDir, 0o755)
	if code, _, errOut := execute(t, "--std", std, "doc", "-o", outDir, mod); code == 0 {
		t.Fatalf("doc -o into a read-only directory should fail, not silently succeed (stderr %q)", errOut)
	}

	after, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != preexisting {
		t.Fatalf("a failed doc write must leave a colliding pre-existing file untouched, got %q", after)
	}
}

// The --html output path has its own write loop and must be equally
// atomic: a colliding file under -o must survive a write that cannot
// complete.
func TestDocHTMLWritePreservesExistingFileOnFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	std, _ := filepath.Abs("../../std")
	mod := t.TempDir()
	os.WriteFile(filepath.Join(mod, "mod.kg"), []byte("Module {\n    name: \"greet\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(mod, "greet.kg"), []byte("/// Says hi.\npub fn hello() -> String {\n    \"hi\"\n}\n"), 0o644)

	outDir := t.TempDir()
	preexisting := "this is real content that must survive a failed doc --html write"
	index := filepath.Join(outDir, "index.html")
	os.WriteFile(index, []byte(preexisting), 0o644)

	if err := os.Chmod(outDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(outDir, 0o755)
	if code, _, errOut := execute(t, "--std", std, "doc", "--html", "-o", outDir, mod); code == 0 {
		t.Fatalf("doc --html -o into a read-only directory should fail, not silently succeed (stderr %q)", errOut)
	}

	after, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != preexisting {
		t.Fatalf("a failed doc --html write must leave a colliding pre-existing file untouched, got %q", after)
	}
	if strings.Contains(string(after), "<html") {
		t.Fatalf("index.html should not have been regenerated: %q", after)
	}
}
