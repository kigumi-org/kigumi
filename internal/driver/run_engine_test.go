package driver_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// TestRunCorpusEngines runs every testdata/run fixture the way `kigumi run`
// does. `--engine vm` either produces the expected output or refuses the
// program by name, and the default engine runs the refused ones on the
// interpreter with the same output.
func TestRunCorpusEngines(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/run/*.txtar")
	if len(files) == 0 {
		t.Fatal("no run fixtures")
	}
	fellBack := 0
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if fixtureEngines(path) == "native" {
				t.Skip("native only")
			}
			root, wantOut, wantErr, wantExit := loadRunFixture(t, path)
			m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: "../../std", Entry: "main/main.kg"})
			if err != nil {
				t.Fatal(err)
			}
			t.Chdir(t.TempDir())
			args := []string{"main/main.kg", "one", "two"}
			var out, errOut bytes.Buffer
			code, err := driver.RunWith(m, driver.RunOptions{Engine: "vm"}, &out, &errOut, args)
			if err != nil {
				if !strings.Contains(err.Error(), "--engine interp") {
					t.Fatalf("vm: %v", err)
				}
				fellBack++
				out.Reset()
				errOut.Reset()
				code, err = driver.RunWith(m, driver.RunOptions{}, &out, &errOut, args)
				if err != nil {
					t.Fatalf("auto: %v", err)
				}
			}
			if out.String() != wantOut {
				t.Errorf("stdout mismatch\n--- got ---\n%s--- want ---\n%s", out.String(), wantOut)
			}
			if wantErr != "" && !strings.Contains(errOut.String(), strings.TrimSpace(wantErr)) {
				t.Errorf("stderr mismatch\n--- got ---\n%s--- want ---\n%s", errOut.String(), wantErr)
			}
			if code != wantExit {
				t.Errorf("exit %d, want %d\nstderr:\n%s", code, wantExit, errOut.String())
			}
		})
	}
	t.Logf("%d fixtures ran on the interpreter", fellBack)
}
