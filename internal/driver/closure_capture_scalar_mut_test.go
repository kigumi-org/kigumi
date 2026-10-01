package driver_test

import (
	"bytes"
	"testing"

	"kigumi/internal/driver"
)

// TestClosureCaptureScalarMutEngines locks interp to vm's output for
// closures writing a captured local through a scalar &mut. Unlike
// TestRunCorpusEngines, it forces interp explicitly, since the usual
// fallback-only wiring let this diverge untested.
func TestClosureCaptureScalarMutEngines(t *testing.T) {
	for _, path := range []string{
		"../../testdata/run/withmut_scalar_closure.txtar",
		"../../testdata/run/withmut_scalar_closures.txtar",
	} {
		path := path
		t.Run(path, func(t *testing.T) {
			root, wantOut, _, wantExit := loadRunFixture(t, path)
			m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: "../../std", Entry: "main/main.kg"})
			if err != nil {
				t.Fatal(err)
			}
			for _, eng := range []string{"vm", "interp"} {
				eng := eng
				t.Run(eng, func(t *testing.T) {
					t.Chdir(t.TempDir())
					var out, errOut bytes.Buffer
					code, err := driver.RunWith(m, driver.RunOptions{Engine: eng}, &out, &errOut, []string{"main/main.kg"})
					if err != nil {
						t.Fatalf("%s: %v\n%s", eng, err, errOut.String())
					}
					if out.String() != wantOut {
						t.Errorf("%s stdout mismatch\n--- got ---\n%s--- want ---\n%s", eng, out.String(), wantOut)
					}
					if code != wantExit {
						t.Errorf("%s exit %d, want %d\nstderr:\n%s", eng, code, wantExit, errOut.String())
					}
				})
			}
		})
	}
}
