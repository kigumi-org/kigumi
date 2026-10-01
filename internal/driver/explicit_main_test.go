package driver_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"kigumi/internal/driver"
)

// TestRunExplicitMain locks in interp/vm/auto parity for the ENT-5 explicit
// entry form: interp.RunEntry must fall back to sem.Result.MainFn() like
// the VM and AOT paths, not only ImplicitMains().
func TestRunExplicitMain(t *testing.T) {
	root := t.TempDir()
	writeEntryScopeModule(t, root, map[string]string{
		"main/main.kg": "fn main() -> Unit! {\n    print(\"hello\")\n}\n",
	})
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Entry: "main/main.kg"})
	if err != nil {
		t.Fatal(err)
	}
	for _, eng := range []string{"vm", "interp", ""} {
		name := eng
		if name == "" {
			name = "auto"
		}
		t.Run(name, func(t *testing.T) {
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

// TestRunExplicitMainWithHost covers the `fn main(host: Host) -> Unit!`
// form (ENT-5's second sanctioned signature) on interp and vm alike.
func TestRunExplicitMainWithHost(t *testing.T) {
	root := t.TempDir()
	writeEntryScopeModule(t, root, map[string]string{
		"main/main.kg": "import os from std/os\n\n" +
			"fn main(host: os.Host) -> Unit! {\n" +
			"    let a = host.args()\n" +
			"    print(\"argc=${a.len()}\")\n" +
			"}\n",
	})
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Entry: "main/main.kg"})
	if err != nil {
		t.Fatal(err)
	}
	for _, eng := range []string{"vm", "interp"} {
		t.Run(eng, func(t *testing.T) {
			t.Chdir(t.TempDir())
			var out, errOut bytes.Buffer
			code, err := driver.RunWith(m, driver.RunOptions{Engine: eng}, &out, &errOut, []string{"one", "two"})
			if err != nil {
				t.Fatalf("%v\n%s", err, errOut.String())
			}
			if out.String() != "argc=2\n" {
				t.Errorf("stdout = %q, want %q (stderr: %s)", out.String(), "argc=2\n", errOut.String())
			}
			if code != 0 {
				t.Errorf("exit = %d, want 0\nstderr:\n%s", code, errOut.String())
			}
		})
	}
}
