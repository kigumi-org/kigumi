package mir_test

import (
	"path/filepath"
	"testing"

	"kigumi/internal/mir"
	"kigumi/internal/testkit"
)

// TestVerifyCorpus runs the verifier over the MIR of every run fixture,
// std included: a program the builder produces without diagnostics has to
// satisfy the contract.
func TestVerifyCorpus(t *testing.T) {
	std := testkit.LoadStd(t, "../../std")
	files, _ := filepath.Glob("../../testdata/run/*.txtar")
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			res, _, _ := checkFixture(t, path, std)
			prog := mir.Build(res)
			if prog.HasErrors() {
				t.Skip("ownership diagnostics")
			}
			if err := prog.Verify(); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}
