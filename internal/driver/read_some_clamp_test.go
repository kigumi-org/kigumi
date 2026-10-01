package driver_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"kigumi/internal/driver"
)

// TestReadSomeNegativeTimeoutClamped checks that a negative timeoutMs is
// clamped to zero before reaching poll(2), which treats any negative
// timeout as "wait forever".
func TestReadSomeNegativeTimeoutClamped(t *testing.T) {
	t.Parallel()
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "main"), 0o755)
	src := "let a = host.stdin().readSome(-5)?\n" +
		"print \"a=${a.len()}\"\n"
	os.WriteFile(filepath.Join(root, "main/main.kg"), []byte(src), 0o644)

	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Entry: "main/main.kg"})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("vm", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		defer w.Close()
		var out, errOut bytes.Buffer
		code, err := driver.RunWith(m, driver.RunOptions{Engine: "vm", Stdin: r}, &out, &errOut, []string{"main/main.kg"})
		if err != nil || code != 0 {
			t.Fatalf("vm: err %v exit %d\n%s", err, code, errOut.String())
		}
		if out.String() != "a=0\n" {
			t.Errorf("vm stdout = %q, want %q", out.String(), "a=0\n")
		}
	})

	t.Run("interp", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		defer w.Close()
		var out, errOut bytes.Buffer
		code, err := driver.RunWith(m, driver.RunOptions{Engine: "interp", Stdin: r}, &out, &errOut, []string{"main/main.kg"})
		if err != nil || code != 0 {
			t.Fatalf("interp: err %v exit %d\n%s", err, code, errOut.String())
		}
		if out.String() != "a=0\n" {
			t.Errorf("interp stdout = %q, want %q", out.String(), "a=0\n")
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
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		defer w.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe)
		cmd.Dir = t.TempDir()
		cmd.Stdin = r
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		if err := cmd.Run(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				t.Fatalf("readSome(-5) hung past the 5s ceiling: poll(2) was not given a clamped timeout\nstderr: %s", errOut.String())
			}
			t.Fatalf("run: %v\nstderr: %s", err, errOut.String())
		}
		if out.String() != "a=0\n" {
			t.Errorf("build stdout = %q, want %q", out.String(), "a=0\n")
		}
	})
}
