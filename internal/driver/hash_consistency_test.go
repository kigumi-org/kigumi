package driver_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"kigumi/internal/driver"
)

// TestHashConsistency runs testdata/run/hash_consistency.txtar on interp
// and vm directly (TestRunCorpusEngines only exercises interp when vm
// refuses a program by name, which never happens here) and on the native
// build, so a hash() drift between any two engines fails here even when
// it would not fail the shared-fixture checks.
func TestHashConsistency(t *testing.T) {
	path := "../../testdata/run/hash_consistency.txtar"
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
