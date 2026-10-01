package driver_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kigumi/internal/driver"
)

const stdinReadAllSrc = `let r = host.stdin().readAll()
match r {
    Ok(s) -> print "OK len=${s.len()}"
    Err(e) -> print "ERR"
}
`

// TestStdinReadAllReportsRealError checks that a real read(2) failure (not
// EOF) surfaces as Err on every engine instead of being folded into an
// empty Ok, the errno-reporting-1 bug: reading from a directory fd fails
// with EISDIR, a genuine kernel error distinct from end-of-input.
func TestStdinReadAllReportsRealError(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "main"), 0o755)
	os.WriteFile(filepath.Join(root, "main/main.kg"), []byte(stdinReadAllSrc), 0o644)

	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Entry: "main/main.kg"})
	if err != nil {
		t.Fatal(err)
	}

	dirStdin := func(t *testing.T) *os.File {
		f, err := os.Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		return f
	}

	for _, engine := range []string{"vm", "interp"} {
		t.Run(engine, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code, err := driver.RunWith(m, driver.RunOptions{Engine: engine, Stdin: dirStdin(t)}, &out, &errOut, []string{"main/main.kg"})
			if err != nil || code != 0 {
				t.Fatalf("%s: err %v exit %d\n%s", engine, err, code, errOut.String())
			}
			if strings.TrimSpace(out.String()) != "ERR" {
				t.Errorf("%s stdout = %q, want %q (a real read error must not look like a clean, empty EOF)", engine, out.String(), "ERR")
			}
		})
	}

	t.Run("build", func(t *testing.T) {
		if driver.CCompiler() == nil {
			t.Skip("no C compiler")
		}
		exe := filepath.Join(t.TempDir(), "prog")
		var buildErr bytes.Buffer
		if ok, err := testBuild(m, exe, &buildErr); err != nil || !ok {
			t.Fatalf("build: %v\n%s", err, buildErr.String())
		}
		cmd := exec.Command(exe)
		cmd.Dir = t.TempDir()
		cmd.Stdin = dirStdin(t)
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		if err := cmd.Run(); err != nil {
			t.Fatalf("run: %v\nstderr: %s", err, errOut.String())
		}
		if strings.TrimSpace(out.String()) != "ERR" {
			t.Errorf("build stdout = %q, want %q", out.String(), "ERR")
		}
	})
}

const stdinReadLineSrc = `let r = host.stdin().readLine()
match r {
    Ok(Some(s)) -> print "SOME len=${s.len()}"
    Ok(None) -> print "NONE"
    Err(e) -> print "ERR"
}
`

// TestStdinReadLineUTF8AndEOF checks readLine's Result[Option[String], Error]
// shape (errno-reporting-5) across engines: invalid UTF-8 on the line must
// fail (utf8-text-3) rather than embedding raw bytes or returning Some("");
// a writer that closes without sending anything must still report a clean
// Ok(None).
func TestStdinReadLineUTF8AndEOF(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "main"), 0o755)
	os.WriteFile(filepath.Join(root, "main/main.kg"), []byte(stdinReadLineSrc), 0o644)

	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Entry: "main/main.kg"})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"invalid_utf8", []byte{0xff, 0xfe, 0xfd, '\n'}, "ERR"},
		{"valid_line", []byte("hello\n"), "SOME len=5"},
		{"closed_writer_no_data", nil, "NONE"},
	}

	for _, engine := range []string{"vm", "interp"} {
		for _, tc := range cases {
			t.Run(engine+"/"+tc.name, func(t *testing.T) {
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer r.Close()
				if len(tc.data) > 0 {
					if _, err := w.Write(tc.data); err != nil {
						t.Fatal(err)
					}
				}
				w.Close()

				var out, errOut bytes.Buffer
				code, err := driver.RunWith(m, driver.RunOptions{Engine: engine, Stdin: r}, &out, &errOut, []string{"main/main.kg"})
				if err != nil || code != 0 {
					t.Fatalf("%s: err %v exit %d\n%s", engine, err, code, errOut.String())
				}
				if strings.TrimSpace(out.String()) != tc.want {
					t.Errorf("%s stdout = %q, want %q", engine, out.String(), tc.want)
				}
			})
		}
	}

	t.Run("build", func(t *testing.T) {
		if driver.CCompiler() == nil {
			t.Skip("no C compiler")
		}
		exe := filepath.Join(t.TempDir(), "prog")
		var buildErr bytes.Buffer
		if ok, err := testBuild(m, exe, &buildErr); err != nil || !ok {
			t.Fatalf("build: %v\n%s", err, buildErr.String())
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer r.Close()
				if len(tc.data) > 0 {
					if _, err := w.Write(tc.data); err != nil {
						t.Fatal(err)
					}
				}
				w.Close()

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, exe)
				cmd.Dir = t.TempDir()
				cmd.Stdin = r
				var out, errOut bytes.Buffer
				cmd.Stdout, cmd.Stderr = &out, &errOut
				if err := cmd.Run(); err != nil {
					t.Fatalf("run: %v\nstderr: %s", err, errOut.String())
				}
				if strings.TrimSpace(out.String()) != tc.want {
					t.Errorf("build stdout = %q, want %q", out.String(), tc.want)
				}
			})
		}
	})
}

const stdinEintrReadAllSrc = `import ffi from std/ffi

type Timeval layout(C) = {
    pub sec i64
    pub usec i64
}

type Itimerval layout(C) = {
    pub interval Timeval
    pub value Timeval
}

extern(C) {
    fn signal(sig: i32, handler: extern(C) fn(i32) -> Unit) -> extern(C) fn(i32) -> Unit
    fn siginterrupt(sig: i32, flag: i32) -> i32
    fn setitimer(which: i32, newValue: *const Itimerval, oldValue: *mut Itimerval) -> i32
}

export(C) {
    pub fn onAlarm(sig: i32) -> Unit {}
}

fn main() -> Unit! {
    unsafe {
        signal(14, onAlarm)
        siginterrupt(14, 1)
    }
    let iv = ffi.CValue.new(Itimerval {
        interval: Timeval { sec: 0, usec: 1000 }
        value: Timeval { sec: 0, usec: 1000 }
    })
    unsafe {
        setitimer(0, ffi.toConst(iv.ptr()), ffi.toMut(ffi.null[Itimerval]()))
    }
    let r = host.stdin().readAll()?
    print "readAll got: ${r.len()} bytes"
}
`

const stdinEintrReadLineSrc = `import ffi from std/ffi

type Timeval layout(C) = {
    pub sec i64
    pub usec i64
}

type Itimerval layout(C) = {
    pub interval Timeval
    pub value Timeval
}

extern(C) {
    fn signal(sig: i32, handler: extern(C) fn(i32) -> Unit) -> extern(C) fn(i32) -> Unit
    fn siginterrupt(sig: i32, flag: i32) -> i32
    fn setitimer(which: i32, newValue: *const Itimerval, oldValue: *mut Itimerval) -> i32
}

export(C) {
    pub fn onAlarm(sig: i32) -> Unit {}
}

fn main() -> Unit! {
    unsafe {
        signal(14, onAlarm)
        siginterrupt(14, 1)
    }
    let iv = ffi.CValue.new(Itimerval {
        interval: Timeval { sec: 0, usec: 1000 }
        value: Timeval { sec: 0, usec: 1000 }
    })
    unsafe {
        setitimer(0, ffi.toConst(iv.ptr()), ffi.toMut(ffi.null[Itimerval]()))
    }
    let r = host.stdin().readLine()?
    print "readLine got: ${r || "<none>"}"
}
`

// TestStdinNativeEINTRRetried is native-only (fd-resource-leaks-1/-3): a
// signal arriving without SA_RESTART must not be mistaken for EOF.
// signal()+siginterrupt(1) plus a 1ms itimer interrupt readAll/readLine's
// blocking read(2) on a FIFO with a delayed writer; the interpreter has no
// FFI and the VM/interp accelerators never call
// raw read(2), so neither needs this coverage.
func TestStdinNativeEINTRRetried(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	std, _ := filepath.Abs("../../std")

	build := func(t *testing.T, src string) string {
		root := t.TempDir()
		os.MkdirAll(filepath.Join(root, "main"), 0o755)
		os.WriteFile(filepath.Join(root, "main/main.kg"), []byte(src), 0o644)
		m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Entry: "main/main.kg"})
		if err != nil {
			t.Fatal(err)
		}
		exe := filepath.Join(t.TempDir(), "prog")
		var buildErr bytes.Buffer
		if ok, err := testBuild(m, exe, &buildErr); err != nil || !ok {
			t.Fatalf("build: %v\n%s", err, buildErr.String())
		}
		return exe
	}

	// runDelayed feeds want on a FIFO after a short delay, while the child
	// (already blocked in read(2)) is under a continuous SIGALRM storm; it
	// retries once on the whole attempt to absorb scheduler noise around
	// the wall-clock delay, per the harness's flakiness note.
	runDelayed := func(t *testing.T, exe string, want string) string {
		t.Helper()
		var lastOut, lastErr string
		for attempt := 0; attempt < 2; attempt++ {
			fifo := filepath.Join(t.TempDir(), "fifo")
			if err := exec.Command("mkfifo", fifo).Run(); err != nil {
				t.Skipf("mkfifo unavailable: %v", err)
			}
			// Pre-open the write end so the child's blocking open(O_RDONLY)
			// on the FIFO returns immediately with the pipe still empty.
			keepOpen, err := os.OpenFile(fifo, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cmd := exec.CommandContext(ctx, exe)
			cmd.Dir = t.TempDir()
			stdinFile, err := os.Open(fifo)
			if err != nil {
				keepOpen.Close()
				cancel()
				t.Fatal(err)
			}
			cmd.Stdin = stdinFile
			var out, errOut bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errOut

			done := make(chan error, 1)
			go func() { done <- cmd.Run() }()
			time.Sleep(300 * time.Millisecond)
			io := []byte("hello world\n")
			keepOpen.Write(io)
			keepOpen.Close()

			runErr := <-done
			stdinFile.Close()
			cancel()
			lastOut, lastErr = strings.TrimSpace(out.String()), errOut.String()
			if runErr == nil && lastOut == want {
				return lastOut
			}
			if ctx.Err() == context.DeadlineExceeded {
				continue // wall-clock flake per the harness note; retry once
			}
		}
		t.Fatalf("got %q (stderr %q), want %q", lastOut, lastErr, want)
		return ""
	}

	t.Run("readAll", func(t *testing.T) {
		exe := build(t, stdinEintrReadAllSrc)
		got := runDelayed(t, exe, "readAll got: 12 bytes")
		if got != "readAll got: 12 bytes" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("readLine", func(t *testing.T) {
		exe := build(t, stdinEintrReadLineSrc)
		got := runDelayed(t, exe, "readLine got: hello world")
		if got != "readLine got: hello world" {
			t.Errorf("got %q", got)
		}
	})
}
