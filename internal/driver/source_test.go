package driver_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// TestSourceCandidates expands Source directories and keeps only the
// candidates whose platform suffix matches the target.
func TestSourceCandidates(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"native\"\n    kigumi: \"0.1\"\n}\nSource { c: \"native\" }\nSource { asm: \"boot\" }\nSource { c: \"one.c\" }\nSource { c: \"only_arm64.c\" }\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("print \"x\"\n"), 0o644)
	os.WriteFile(filepath.Join(root, "one.c"), nil, 0o644)
	os.WriteFile(filepath.Join(root, "only_arm64.c"), nil, 0o644)
	os.MkdirAll(filepath.Join(root, "native"), 0o755)
	os.MkdirAll(filepath.Join(root, "boot"), 0o755)
	for _, f := range []string{"native/x.c", "native/x_linux.c", "native/x_windows.c", "native/x_freestanding.c", "native/notes.txt", "boot/start_amd64.s", "boot/start_arm64.s", "boot/common.S", "boot/x.c"} {
		os.WriteFile(filepath.Join(root, f), nil, 0o644)
	}
	target, _ := driver.ParseTarget("x86_64-linux", "")
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range m.Sources {
		rel, _ := filepath.Rel(root, s)
		got = append(got, filepath.ToSlash(rel))
	}
	slices.Sort(got)
	want := []string{"boot/common.S", "boot/start_amd64.s", "native/x.c", "native/x_linux.c", "one.c"}
	if !slices.Equal(got, want) {
		t.Fatalf("sources for x86_64-linux: %s", strings.Join(got, " "))
	}
}
