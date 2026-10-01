package driver_test

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"kigumi/internal/driver"
)

// watchReadyWriter forwards bytes to w and closes ready the first time
// "watching" appears, so a caller can block on sig.watch(...) actually
// running instead of guessing a sleep. mu also guards String() so a
// timeout branch can read what's been written so far without racing the
// write still in flight on the RunWith goroutine.
type watchReadyWriter struct {
	w     io.Writer
	ready chan struct{}
	once  sync.Once
	mu    sync.Mutex
	seen  []byte
}

func (r *watchReadyWriter) Write(p []byte) (int, error) {
	n, err := r.w.Write(p)
	r.mu.Lock()
	r.seen = append(r.seen, p...)
	watching := bytes.Contains(r.seen, []byte("watching"))
	r.mu.Unlock()
	if watching {
		r.once.Do(func() { close(r.ready) })
	}
	return n, err
}

func (r *watchReadyWriter) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.seen)
}

// signalSrc watches SIGINT and polls pending() until it fires or the
// 3-second ceiling passes, printing whether it was ever observed.
const signalSrc = `import os from std/os
import time from std/time

let sig = host.signals()
sig.watch(os.Signal.Interrupt)?
print "watching"
let mut n = 0
let mut got = false
for n < 3000 {
    if sig.pending(os.Signal.Interrupt) {
        got = true
        break
    }
    time.sleep(1)
    n = n + 1
}
print "pending=${got}"
`

// TestSignalWatchPending sends a real SIGINT (syscall.Kill) to itself for
// the interpreter and the VM, and to the child process for a native build,
// and checks watch/pending observes it without crashing: the C-side handler
// (rt_signal.c) only sets a flag, so it never re-enters the refcounted
// runtime the way running arbitrary Kigumi from signal context would.
func TestSignalWatchPending(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "main"), 0o755)
	os.WriteFile(filepath.Join(root, "main/main.kg"), []byte(signalSrc), 0o644)

	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Entry: "main/main.kg"})
	if err != nil {
		t.Fatal(err)
	}

	runEngine := func(t *testing.T, engine string) {
		var out, errOut bytes.Buffer
		watched := &watchReadyWriter{w: &out, ready: make(chan struct{})}
		done := make(chan struct{})
		var code int
		var runErr error
		go func() {
			code, runErr = driver.RunWith(m, driver.RunOptions{Engine: engine}, watched, &errOut, []string{"main/main.kg"})
			close(done)
		}()
		select {
		case <-watched.ready:
		case <-done:
			// Exited before ever printing "watching": fall through so the
			// mismatch surfaces below instead of hanging on a signal that
			// would now have nothing to interrupt.
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: never printed \"watching\"\n%s", engine, watched.String())
		}
		if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
			t.Fatal(err)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: watch/pending never returned\n%s", engine, watched.String())
		}
		if runErr != nil || code != 0 {
			t.Fatalf("%s: err %v exit %d\n%s", engine, runErr, code, errOut.String())
		}
		if !strings.Contains(out.String(), "pending=true") {
			t.Errorf("%s stdout = %q, want it to contain pending=true", engine, out.String())
		}
	}
	t.Run("interp", func(t *testing.T) { runEngine(t, "interp") })
	t.Run("vm", func(t *testing.T) { runEngine(t, "vm") })

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
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		r := bufio.NewReader(stdout)
		line, err := r.ReadString('\n')
		if err != nil || strings.TrimSpace(line) != "watching" {
			t.Fatalf("build: first line = %q, err %v", line, err)
		}
		if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
			t.Fatal(err)
		}
		rest, _ := io.ReadAll(r)
		if err := cmd.Wait(); err != nil {
			t.Fatalf("build: %v\n%s", err, rest)
		}
		if !strings.Contains(string(rest), "pending=true") {
			t.Errorf("build stdout = %q, want it to contain pending=true", string(rest))
		}
	})
}

// signalTestSrc calls Signals.watch from a test block, then checks that
// pending() still reports false: watch must have refused before it ever
// reached os/signal.Notify, not merely failed after installing the handler.
// `host` is entry-file-only, so the tests sit in the entry file
// itself rather than a separate _test.kg.
const signalTestSrc = `import os from std/os
import test from std/test

test "signal watch is refused under kigumi test" {
    let sig = host.signals()
    sig.watch(os.Signal.Interrupt)?
}

test "no signal handler was installed" {
    test.expectEq(host.signals().pending(os.Signal.Interrupt), false)?
}
`

// TestSignalDeniedUnderKigumiTest checks a host-effect leak: `kigumi test`
// runs test blocks in the same process as the CLI, so a real
// sigaction/os-signal.Notify installed from a test would hijack the
// runner's own Ctrl-C handling for the rest of the invocation.
// Signals.watch must refuse instead of installing anything.
func TestSignalDeniedUnderKigumiTest(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "main"), 0o755)
	os.WriteFile(filepath.Join(root, "main/main.kg"), []byte(signalTestSrc), 0o644)

	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Test: true, Entry: "main/main.kg"})
	if err != nil {
		t.Fatal(err)
	}

	runEngine := func(t *testing.T, engine string) {
		var out, errOut bytes.Buffer
		code, err := driver.RunTestsWith(m, driver.RunOptions{Engine: engine}, &out, &errOut)
		if err != nil {
			t.Fatalf("%s: %v\n%s%s", engine, err, out.String(), errOut.String())
		}
		if code == 0 {
			t.Fatalf("%s: want a failed test, got exit 0\n%s%s", engine, out.String(), errOut.String())
		}
		if !strings.Contains(out.String(), `FAIL "signal watch is refused under kigumi test"`) {
			t.Errorf("%s stdout = %q, want the watch test to FAIL", engine, out.String())
		}
		if !strings.Contains(errOut.String(), "not available under `kigumi test`") {
			t.Errorf("%s stderr = %q, want the refusal message", engine, errOut.String())
		}
		if !strings.Contains(out.String(), `ok   "no signal handler was installed"`) {
			t.Errorf("%s stdout = %q, want pending() to still report false", engine, out.String())
		}
	}
	t.Run("interp", func(t *testing.T) { runEngine(t, "interp") })
	t.Run("vm", func(t *testing.T) { runEngine(t, "vm") })
}
