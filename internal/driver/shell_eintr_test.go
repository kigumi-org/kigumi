package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"kigumi/internal/driver"
)

// A no-op SIGALRM handler plus siginterrupt(3) makes blocking syscalls
// return EINTR like a real unrelated signal landing mid-syscall; the
// subshell traps ALRM (`trap "" ALRM`) so only this process's own retry
// loop is exercised, not sh's default-terminate disposition.
const shellEintrReadSrc = `import shell from std/shell
import coreutil from std/shell/coreutil
import text from std/text

extern(C) {
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

let sh = host.shell()
let plan = $"sh -c 'trap \"\" ALRM; for i in $(seq 1 20); do sleep 0.05; echo line$i; done'" |> coreutil.grep("line")
let captured = (plan |> shell.capture(sh))?
let out = String.fromBytes(captured.stdout)?
let mut n = 0
for line in text.lines(out) {
    n = n + 1
}
print "matched=${n}"
`

// shell.run's writeAll(1, ...) puts captured stdout onto the real fd 1 in
// one blocking loop with no poll() in front of it, unlike drain()'s
// readSome; a slow reader on the other end of this process's own stdout
// forces write() to block long enough for the storm to land inside it.
const shellEintrWriteSrc = `import shell from std/shell

extern(C) {
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

let sh = host.shell()
let plan = $"sh -c 'trap \"\" ALRM; head -c 2000000 /dev/zero | tr \\0 A'"
let status = (plan |> shell.run(sh))?
shell.check(status)?
`

func buildShellEintrProg(t *testing.T, src string) string {
	t.Helper()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"eintrprobe\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "prog")
	var buildErr bytes.Buffer
	if ok, err := testBuild(m, exe, &buildErr); err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}
	return exe
}

// stormAlarm sends SIGALRM to the process group of pid every 5ms until stop
// is closed, so any blocking syscall the group's leader is in has a high
// chance of being interrupted at least once.
func stormAlarm(pid int, stop <-chan struct{}) {
	t := time.NewTicker(5 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			_ = syscall.Kill(-pid, syscall.SIGALRM)
		}
	}
}

// TestShellReadAllRetriesOnEINTR covers fd-resource-leaks-2: readAll (the
// in-process grep filter's stdin slurp) must retry on EINTR instead of
// treating it as EOF, even when an unrelated signal lands mid-read.
func TestShellReadAllRetriesOnEINTR(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix signals only")
	}
	exe := buildShellEintrProg(t, shellEintrReadSrc)

	cmd := exec.Command(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); stormAlarm(cmd.Process.Pid, stop) }()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		close(stop)
		wg.Wait()
		if err != nil {
			t.Fatalf("run: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
		}
	case <-time.After(15 * time.Second):
		close(stop)
		wg.Wait()
		_ = cmd.Process.Kill()
		<-done // join cmd.Wait()'s copy goroutines before reading out/errOut
		t.Fatalf("timed out; stdout=%s stderr=%s", out.String(), errOut.String())
	}

	got := strings.TrimSpace(out.String())
	if got != "matched=20" {
		t.Errorf("under a SIGALRM storm, got %q, want \"matched=20\" (readAll truncated on EINTR)", got)
	}
}

// TestShellRunWriteAllRetriesOnEINTR covers the writeAll sibling of
// fd-resource-leaks-2: run() must retry fd 1 writes through EINTR instead
// of returning early after the first short write under a signal storm.
func TestShellRunWriteAllRetriesOnEINTR(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix signals only")
	}
	exe := buildShellEintrProg(t, shellEintrWriteSrc)

	cmd := exec.Command(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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
	go func() { defer wg.Done(); stormAlarm(cmd.Process.Pid, stop) }()

	// Let the pipe's kernel buffer fill and writeAll's loop start blocking
	// on write() before this test ever reads a byte, so the storm has a
	// real blocked write() to land inside rather than an idle process.
	time.Sleep(1 * time.Second)

	read := make(chan int, 1)
	go func() {
		buf := make([]byte, 1<<20)
		n := 0
		for {
			k, err := stdout.Read(buf)
			n += k
			if err != nil {
				read <- n
				return
			}
		}
	}()

	var total int
	select {
	case total = <-read:
	case <-time.After(15 * time.Second):
		close(stop)
		wg.Wait()
		_ = cmd.Process.Kill()
		_ = cmd.Wait() // join the stderr copy goroutine before reading errOut
		t.Fatalf("timed out reading stdout; stderr=%s", errOut.String())
	}
	close(stop)
	wg.Wait()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("run: %v\nstderr=%s", err, errOut.String())
	}
	if total != 2000000 {
		t.Errorf("under a SIGALRM storm, captured %d bytes, want 2000000 (writeAll returned early on EINTR)", total)
	}
}
