package driver_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"kigumi/internal/driver"
)

// The alarm handler is installed before readAll so a SIGALRM storm can't
// kill the process (default disposition) before print ever runs; readAll's
// own EINTR retry absorbs any storm hits during the read.
const rtSysWriteEintrSrc = `extern(C) {
    fn signal(sig: i32, handler: extern(C) fn(i32) -> Unit) -> usize
    fn siginterrupt(sig: i32, flag: i32) -> i32
}

export(C) {
    pub fn onAlarm(sig: i32) -> Unit {
    }
}

_ = unsafe {
    signal(14, onAlarm)
}
_ = unsafe {
    siginterrupt(14, 1)
}

let acc = host.stdin().readAll()?

print acc
`

// stormSignal sends sig directly to pid (not its process group: the target
// here is a single native process with no children) every 5ms until stop is
// closed.
func stormSignal(pid int, sig syscall.Signal, stop <-chan struct{}) {
	t := time.NewTicker(5 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			_ = syscall.Kill(pid, sig)
		}
	}
}

// TestRtSysWriteRetriesOnEINTR checks a SIGALRM storm (siginterrupt disables
// SA_RESTART, so blocking syscalls return EINTR instead of auto-restarting)
// landing mid print() of several MB to a slow reader does not truncate
// output.
func TestRtSysWriteRetriesOnEINTR(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix signals only")
	}
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"writeeintrprobe\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte(rtSysWriteEintrSrc), 0o644)
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "prog")
	var buildErr bytes.Buffer
	if ok, err := testBuild(m, exe, &buildErr); err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}

	const n = 5000000
	payload := bytes.Repeat([]byte{'A'}, n)
	want := n + 1 // print adds a trailing newline

	// Retried once: real wall-clock timing (a 1s settle, a 15s deadline)
	// alongside a parallel full-suite run on the same machine means a
	// slow scheduler tick can cost the storm its window on its own.
	var gotOut []byte
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		cmd := exec.Command(exe)
		cmd.Dir = t.TempDir()
		cmd.Stdin = bytes.NewReader(payload)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		var errOut bytes.Buffer
		cmd.Stderr = &errOut
		if err := cmd.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}

		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); stormSignal(cmd.Process.Pid, syscall.SIGALRM, stop) }()

		// Let the pipe's kernel buffer fill and print's write() loop start
		// blocking before this test ever reads a byte, so the storm has a
		// real blocked write() to land inside rather than an idle process.
		time.Sleep(1 * time.Second)

		read := make(chan []byte, 1)
		go func() {
			buf := new(bytes.Buffer)
			chunk := make([]byte, 1<<20)
			for {
				k, err := stdout.Read(chunk)
				buf.Write(chunk[:k])
				if err != nil {
					read <- buf.Bytes()
					return
				}
			}
		}()

		var out []byte
		timedOut := false
		select {
		case out = <-read:
		case <-time.After(15 * time.Second):
			timedOut = true
		}
		close(stop)
		wg.Wait()
		if timedOut {
			_ = cmd.Process.Kill()
			_ = cmd.Wait() // join the stderr copy goroutine before reading errOut
			lastErr = fmt.Errorf("timed out reading stdout; stderr=%s", errOut.String())
			continue
		}
		if err := cmd.Wait(); err != nil {
			lastErr = fmt.Errorf("run: %v\nstderr=%s", err, errOut.String())
			continue
		}
		gotOut = out
		if len(gotOut) == want {
			break
		}
		lastErr = fmt.Errorf("under a SIGALRM storm, got %d bytes, want %d (rt_sys_write returned early on EINTR)", len(gotOut), want)
	}

	if lastErr != nil && len(gotOut) != want {
		t.Fatal(lastErr)
	}
	if len(gotOut) != want {
		t.Fatalf("got %d bytes, want %d", len(gotOut), want)
	}
	for i := 0; i < n; i++ {
		if gotOut[i] != 'A' {
			t.Fatalf("byte %d = %q, want 'A' (retry corrupted output instead of resuming at the right offset)", i, gotOut[i])
		}
	}
	if gotOut[n] != '\n' {
		t.Fatalf("trailing byte = %q, want newline", gotOut[n])
	}
}
