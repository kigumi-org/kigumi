package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// TestAllocatorNoneProfile carries `--allocator none` into the checker.
func TestAllocatorNoneProfile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"bare\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("let xs = Array.of(1)\n"), 0o644)
	std, _ := filepath.Abs("../../std")
	target := driver.HostTarget()
	target.Allocator = "none"
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasErrors() || !strings.Contains(res.Render(), "needs the default allocator") {
		t.Fatalf("expected the allocator diagnostic:\n%s", res.Render())
	}
}
