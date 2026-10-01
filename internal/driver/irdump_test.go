package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

func TestIRDump(t *testing.T) {
	t.Parallel()
	out := os.Getenv("IRDUMP_DIR")
	if out == "" {
		t.Skip()
	}
	std, _ := filepath.Abs("../../std")
	files, _ := filepath.Glob("../../testdata/run/*.txtar")
	for _, path := range files {
		data, _ := os.ReadFile(path)
		text := string(data)
		i := strings.Index(text, "-- src --\n")
		j := strings.Index(text, "\n-- ")
		if i < 0 {
			continue
		}
		rest := text[i+len("-- src --\n"):]
		if k := strings.Index(rest, "\n-- "); k >= 0 {
			rest = rest[:k+1]
		}
		_ = j
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"ir\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
		os.WriteFile(filepath.Join(root, "main.kg"), []byte(rest), 0o644)
		m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
		if err != nil {
			continue
		}
		var stderr bytes.Buffer
		ir, ok, err := driver.Compile(m, &stderr)
		if err != nil || !ok {
			continue
		}
		os.WriteFile(filepath.Join(out, filepath.Base(path)+".ll"), []byte(ir), 0o644)
	}
}
