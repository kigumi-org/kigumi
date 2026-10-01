package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

func TestDependencies(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "lib")
	app := filepath.Join(root, "app")
	os.MkdirAll(filepath.Join(lib, "util"), 0o755)
	os.MkdirAll(app, 0o755)
	os.WriteFile(filepath.Join(lib, "mod.kg"), []byte("Module {\n    name: \"lib\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(lib, "lib.kg"), []byte("pub fn twice(x: Int) -> Int {\n    x * 2\n}\n\nfn hidden() -> Int {\n    1\n}\n"), 0o644)
	os.WriteFile(filepath.Join(lib, "util", "util.kg"), []byte("pub fn thrice(x: Int) -> Int {\n    x * 3\n}\n"), 0o644)
	os.WriteFile(filepath.Join(app, "mod.kg"), []byte("Module {\n    name: \"app\"\n    kigumi: \"0.1\"\n}\nRequire { name: \"lib\", version: \"v1.0.0\", url: \"https://example.invalid/lib.git\" }\n"), 0o644)
	// lib lives next door while both are developed.
	os.WriteFile(filepath.Join(app, "mod.local.kg"), []byte("Replace { name: \"lib\", path: \"../lib\" }\n"), 0o644)
	os.WriteFile(filepath.Join(app, "main.kg"), []byte("import {twice} from lib\nimport util from lib/util\nprint \"${twice(2)} ${util.thrice(2)}\"\n"), 0o644)
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(app, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	if p := m.Packages["lib"]; p == nil || p.Module != "lib" {
		t.Fatalf("dependency package not loaded under its name: %+v", m.Order)
	}
	var out, errOut bytes.Buffer
	if code, err := driver.Run(m, &out, &errOut, []string{"app"}); err != nil || code != 0 || out.String() != "4 6\n" {
		t.Fatalf("run: %v exit %d out %q err %q", err, code, out.String(), errOut.String())
	}

	os.WriteFile(filepath.Join(app, "main.kg"), []byte("import {hidden} from lib\nprint \"${hidden()}\"\n"), 0o644)
	m, _ = driver.LoadModule(app, driver.LoadOptions{StdRoot: std})
	res, _ := driver.Check(m)
	if !res.HasErrors() || !strings.Contains(res.Render(), "hidden") {
		t.Fatalf("private dependency function should be rejected:\n%s", res.Render())
	}

	os.Remove(filepath.Join(app, "mod.local.kg"))
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if _, err := driver.LoadModule(app, driver.LoadOptions{StdRoot: std}); err == nil || !strings.Contains(err.Error(), "kigumi get") {
		t.Fatalf("missing dependency should ask for kigumi get: %v", err)
	}
}
