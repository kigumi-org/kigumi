package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

const direntLossyUtf8Src = `import fs from std/fs

let files = host.files()
let dirPath = fs.Path.fromString("sub")?
match files.list(dirPath) {
    Ok(names) -> {
        if names.get(0) is Some(p) {
            print "name=${p.toString()}"
        } else {
            print "empty"
        }
    }
    Err(e) -> print "err: ${e.message()}"
}
`

// TestDirentLossyUtf8 covers ffi-string-utf8: a
// directory entry name that isn't valid UTF-8 (an arbitrary byte string on
// Linux) must list as U+FFFD in place of the bad byte instead of building a
// String that fails its UTF-8 invariant. VM and interp back Files.list with
// Go's os.ReadDir, native with readdir(3) through ffi.stringLossy; all three
// must land on the same replacement.
func TestDirentLossyUtf8(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	sub := filepath.Join(root, "main", "sub")
	os.MkdirAll(sub, 0o755)
	// A single 0xFF byte: not a valid UTF-8 lead byte on its own, so it
	// becomes exactly one U+FFFD.
	invalidName := string([]byte{0xff})
	if err := os.WriteFile(filepath.Join(sub, invalidName), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "main/main.kg"), []byte(direntLossyUtf8Src), 0o644)

	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Entry: "main/main.kg"})
	if err != nil {
		t.Fatal(err)
	}

	want := "name=sub/�"
	for _, engine := range []string{"vm", "interp"} {
		t.Run(engine, func(t *testing.T) {
			t.Chdir(filepath.Join(root, "main"))
			var out, errOut bytes.Buffer
			code, err := driver.RunWith(m, driver.RunOptions{Engine: engine}, &out, &errOut, []string{"main/main.kg"})
			if err != nil || code != 0 {
				t.Fatalf("%s: err %v exit %d\n%s", engine, err, code, errOut.String())
			}
			if got := strings.TrimSpace(out.String()); got != want {
				t.Errorf("%s stdout = %q, want %q", engine, got, want)
			}
		})
	}

	t.Run("native", func(t *testing.T) {
		if driver.CCompiler() == nil {
			t.Skip("no C compiler")
		}
		exe := filepath.Join(t.TempDir(), "prog")
		var buildErr bytes.Buffer
		if ok, err := testBuild(m, exe, &buildErr); err != nil || !ok {
			t.Fatalf("build: %v\n%s", err, buildErr.String())
		}
		cmd := exec.Command(exe)
		cmd.Dir = filepath.Join(root, "main")
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		if err := cmd.Run(); err != nil {
			t.Fatalf("run: %v\nstderr: %s", err, errOut.String())
		}
		if got := strings.TrimSpace(out.String()); got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})
}
