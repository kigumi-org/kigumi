package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"kigumi/internal/driver"
)

// One build invocation checks, lowers and emits once.
func TestSinglePhaseBuild(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"once\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("fn twice(n: Int) -> Int {\n    n * 2\n}\n\nprint \"${twice(21)}\"\n"), 0o644)
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if _, ok, err := driver.Compile(m, &stderr); err != nil || !ok {
		t.Fatalf("compile: %v\n%s", err, stderr.String())
	}
	if got := m.Stats(); got != (driver.Stats{Check: 1, MIR: 1, Emit: 1}) {
		t.Errorf("phase runs: %+v", got)
	}
}
