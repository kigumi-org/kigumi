package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kigumi/internal/driver"
)

const hostEnvArgsUtf8Src = `let e = host.env("KIGUMI_UTF8_TEST_VAR")
match e {
    Some(s) -> print "env=Some(${s.len()})"
    None -> print "env=None"
}
let a = host.args().get(1)
match a {
    Some(s) -> print "arg=Some(${s.len()})"
    None -> print "arg=None"
}
`

// TestHostEnvArgsRejectInvalidUtf8 covers utf8-text-2: an environment
// variable or argv entry that isn't valid UTF-8 (arbitrary bytes on Unix)
// must fold into None on every engine, not build a String that fails its
// UTF-8 invariant.
func TestHostEnvArgsRejectInvalidUtf8(t *testing.T) {
	invalid := string([]byte{0xff, 0xfe, 0xfd})
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "main"), 0o755)
	os.WriteFile(filepath.Join(root, "main/main.kg"), []byte(hostEnvArgsUtf8Src), 0o644)

	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Entry: "main/main.kg"})
	if err != nil {
		t.Fatal(err)
	}

	for _, engine := range []string{"vm", "interp"} {
		t.Run(engine, func(t *testing.T) {
			t.Setenv("KIGUMI_UTF8_TEST_VAR", invalid)
			var out, errOut bytes.Buffer
			args := []string{"main/main.kg", invalid}
			code, err := driver.RunWith(m, driver.RunOptions{Engine: engine}, &out, &errOut, args)
			if err != nil || code != 0 {
				t.Fatalf("%s: err %v exit %d\nstderr: %s", engine, err, code, errOut.String())
			}
			want := "env=None\narg=None\n"
			if out.String() != want {
				t.Errorf("%s stdout = %q, want %q (invalid UTF-8 must not reach String unvalidated)", engine, out.String(), want)
			}
		})
	}

	t.Run("valid_utf8_still_works", func(t *testing.T) {
		t.Setenv("KIGUMI_UTF8_TEST_VAR", "hello")
		for _, engine := range []string{"vm", "interp"} {
			var out, errOut bytes.Buffer
			args := []string{"main/main.kg", "hi"}
			code, err := driver.RunWith(m, driver.RunOptions{Engine: engine}, &out, &errOut, args)
			if err != nil || code != 0 {
				t.Fatalf("%s: err %v exit %d\nstderr: %s", engine, err, code, errOut.String())
			}
			want := "env=Some(5)\narg=Some(2)\n"
			if out.String() != want {
				t.Errorf("%s stdout = %q, want %q", engine, out.String(), want)
			}
		}
	})

	t.Run("build", func(t *testing.T) {
		if driver.CCompiler() == nil {
			t.Skip("no C compiler")
		}
		exe := filepath.Join(t.TempDir(), "prog")
		var buildErr bytes.Buffer
		if ok, err := testBuild(m, exe, &buildErr); err != nil || !ok {
			t.Fatalf("build: %v\n%s", err, buildErr.String())
		}

		run := func(env []string, arg string) string {
			cmd := exec.Command(exe, arg)
			cmd.Dir = t.TempDir()
			cmd.Env = env
			var out, errOut bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errOut
			if err := cmd.Run(); err != nil {
				t.Fatalf("run: %v\nstderr: %s", err, errOut.String())
			}
			return out.String()
		}

		t.Run("invalid", func(t *testing.T) {
			got := run(append(os.Environ(), "KIGUMI_UTF8_TEST_VAR="+invalid), invalid)
			want := "env=None\narg=None\n"
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
		t.Run("valid", func(t *testing.T) {
			got := run(append(os.Environ(), "KIGUMI_UTF8_TEST_VAR=hello"), "hi")
			want := "env=Some(5)\narg=Some(2)\n"
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	})
}
