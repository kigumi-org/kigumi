package driver_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"kigumi/internal/driver"
)

// TestFloatTotalEqOrd runs testdata/run/float_total_eq_ord.txtar on interp,
// vm and the native build directly, so the Eq/Ord/Hash protocol's total
// order for NaN and -0.0 agrees across
// all three engines even when TestRunCorpusEngines would not catch a drift
// (it only runs interp when vm refuses the program by name). It also pins
// that a generic function over T: Eq/Ord always dispatches through
// equals/compareTo (a pre-existing witness rule), so a
// float compared inside such a function uses the total order even where
// the same comparison written directly against f64 stays IEEE.
func TestFloatTotalEqOrd(t *testing.T) {
	path := "../../testdata/run/float_total_eq_ord.txtar"
	root, wantOut, _, _ := loadRunFixture(t, path)
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: "../../std", Entry: "main/main.kg"})
	if err != nil {
		t.Fatal(err)
	}

	run := func(t *testing.T, engine string) string {
		t.Chdir(t.TempDir())
		var out, errOut bytes.Buffer
		code, err := driver.RunWith(m, driver.RunOptions{Engine: engine}, &out, &errOut, []string{"main/main.kg"})
		if err != nil || code != 0 {
			t.Fatalf("%s: err %v exit %d\n%s", engine, err, code, errOut.String())
		}
		return out.String()
	}

	vmOut := run(t, "vm")
	if vmOut != wantOut {
		t.Errorf("vm stdout mismatch\n--- got ---\n%s--- want ---\n%s", vmOut, wantOut)
	}
	interpOut := run(t, "interp")
	if interpOut != wantOut {
		t.Errorf("interp stdout mismatch\n--- got ---\n%s--- want ---\n%s", interpOut, wantOut)
	}

	if driver.CCompiler() == nil {
		t.Skip("no C compiler for the native build")
	}
	exe := filepath.Join(t.TempDir(), "prog")
	var buildErr bytes.Buffer
	if ok, err := testBuild(m, exe, &buildErr); err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}
	cmd := exec.Command(exe)
	cmd.Dir = t.TempDir()
	var nativeOut bytes.Buffer
	cmd.Stdout = &nativeOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("native run: %v", err)
	}
	if nativeOut.String() != wantOut {
		t.Errorf("native stdout mismatch\n--- got ---\n%s--- want ---\n%s", nativeOut.String(), wantOut)
	}
}
