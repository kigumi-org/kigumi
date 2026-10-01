package shared

import (
	"os"
	"path/filepath"
	"testing"
)

// A package directory inside a module names its entry file, so
// `kigumi build cmd/app` works next to `cmd/tool`; module roots and plain
// library directories pass through unchanged.
func TestPackageEntry(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"cmd/app/main.kg", "cmd/tool/tool.kg", "lib/a.kg", "lib/b.kg"} {
		os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755)
		os.WriteFile(filepath.Join(root, p), []byte("print 1\n"), 0o644)
	}
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"m\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	for dir, want := range map[string]string{
		"cmd/app":  "cmd/app/main.kg",
		"cmd/tool": "cmd/tool/tool.kg",
		"lib":      "lib",
		".":        ".",
	} {
		got := packageEntry(filepath.Join(root, dir))
		if got != filepath.Join(root, want) {
			t.Errorf("packageEntry(%s) = %s, want %s", dir, got, filepath.Join(root, want))
		}
	}
}
