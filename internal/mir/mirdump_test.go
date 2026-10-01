package mir_test

import (
	"os"
	"path/filepath"
	"testing"

	"kigumi/internal/mir"
	"kigumi/internal/testkit"
)

// TestMirDump writes the MIR of every run fixture under MIRDUMP_DIR, so a
// change to the lowering can be compared byte for byte.
func TestMirDump(t *testing.T) {
	out := os.Getenv("MIRDUMP_DIR")
	if out == "" {
		t.Skip()
	}
	std := testkit.LoadStd(t, "../../std")
	files, _ := filepath.Glob("../../testdata/run/*.txtar")
	for _, path := range files {
		res, _, _ := checkFixture(t, path, std)
		prog := mir.Build(res)
		os.WriteFile(filepath.Join(out, filepath.Base(path)+".mir"), []byte(prog.Dump()+prog.Render()), 0o644)
	}
}
