package driver_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"kigumi/internal/driver"
)

// TestRunExplicitMainNoExplicitEntry covers `kigumi run <dir>` (no filename
// argument): Module.defaultEntry must recognize an explicit
// `fn main() -> Unit!` root file as the entry, not only a script-shaped
// one, on every engine.
func TestRunExplicitMainNoExplicitEntry(t *testing.T) {
	root := t.TempDir()
	writeEntryScopeModule(t, root, map[string]string{
		"main.kg": "fn main() -> Unit! {\n    print(\"hello\")\n}\n",
	})
	std, _ := filepath.Abs("../../std")
	for _, eng := range []string{"vm", "interp", ""} {
		name := eng
		if name == "" {
			name = "auto"
		}
		t.Run(name, func(t *testing.T) {
			m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
			if err != nil {
				t.Fatal(err)
			}
			t.Chdir(t.TempDir())
			var out, errOut bytes.Buffer
			code, err := driver.RunWith(m, driver.RunOptions{Engine: eng}, &out, &errOut, nil)
			if err != nil {
				t.Fatalf("%v\n%s", err, errOut.String())
			}
			if out.String() != "hello\n" {
				t.Errorf("stdout = %q, want %q (stderr: %s)", out.String(), "hello\n", errOut.String())
			}
			if code != 0 {
				t.Errorf("exit = %d, want 0\nstderr:\n%s", code, errOut.String())
			}
		})
	}
}

// TestCheckExplicitMainCmdPackage covers `kigumi check` on a whole
// build-program-shaped module (no explicit entry): Module.semModule's cmd/
// loop must give a cmd/ package written as an explicit `fn main` its own
// entry too, not only script-shaped ones.
func TestCheckExplicitMainCmdPackage(t *testing.T) {
	root := t.TempDir()
	writeEntryScopeModule(t, root, map[string]string{
		"mod.kg":               "Module {\n    name: \"cmdmain\"\n    kigumi: \"0.1\"\n}\n",
		"cmd/script/main.kg":   "print \"from script\"\n",
		"cmd/explicit/main.kg": "fn main() -> Unit! {\n    print(\"from explicit\")\n}\n",
	})
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.HasErrors() {
		t.Fatalf("both cmd/ entry styles should check clean:\n%s", res.Render())
	}
}
