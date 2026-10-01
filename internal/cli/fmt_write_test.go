package cli_test

import (
	"os"
	"path/filepath"
	"testing"
)

// `kigumi fmt -w` must write atomically, like BUILD-005's build-graph
// output, preserving the source's mode and leaving it untouched on failure.
func TestFmtWriteIsAtomicAndPreservesMode(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "tool.kg")
	if err := os.WriteFile(file, []byte("#!/usr/bin/env kigumi\nlet  x = 1\nprint \"v=${x}\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if code, _, errOut := execute(t, "fmt", "-w", file); code != 0 {
		t.Fatalf("fmt -w: exit %d, stderr %q", code, errOut)
	}
	fi, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("executable bit lost: mode is %v", fi.Mode().Perm())
	}
	formatted, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	if code, _, _ := execute(t, "fmt", "-w", file); code == 0 {
		t.Fatal("fmt -w into a read-only directory should fail, not silently succeed")
	}
	after, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(formatted) {
		t.Fatalf("a failed write must leave the original content untouched, got %q", after)
	}
}
