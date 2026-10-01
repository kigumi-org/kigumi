package driver_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

const shellFdSafetyBadRedirect = `import shell from std/shell
import fs from std/fs

fn countFds() -> Int! {
    let files = host.files()
    let p = fs.Path.fromString("/proc/self/fd")?
    let entries = files.list(p)?
    entries.len().toInt() || -1
}

let sh = host.shell()
let before = countFds() || -1
let mut errs = 0
for i in 0..5 {
    match $"printf hi > /nonexistent-dir-kigumi-fdsafety/out" |> shell.capture(sh) {
        Ok(_) -> {}
        Err(_) -> { errs = errs + 1 }
    }
}
let after = countFds() || -1
print "errs=${errs} before=${before} after=${after}"
`

const shellFdSafetyLowNofile = `import shell from std/shell
import fs from std/fs

fn countFds() -> Int! {
    let files = host.files()
    let p = fs.Path.fromString("/proc/self/fd")?
    let entries = files.list(p)?
    entries.len().toInt() || -1
}

let sh = host.shell()
let before = countFds() || -1
let mut msgs: Array[String] = Array.empty()
for i in 0..5 {
    match $"echo hi" |> shell.capture(sh) {
        Ok(_) -> msgs.push("ok")
        Err(f) -> msgs.push(f.message())
    }
}
let after = countFds() || -1
print "before=${before} after=${after}"
for m in msgs {
    print "msg=${m}"
}
`

const shellFdSafetyPipeSetup = `import shell from std/shell
import fs from std/fs

fn countFds() -> Int! {
    let files = host.files()
    let p = fs.Path.fromString("/proc/self/fd")?
    let entries = files.list(p)?
    entries.len().toInt() || -1
}

let sh = host.shell()
let before = countFds() || -1
let mut msgs: Array[String] = Array.empty()
for i in 0..5 {
    match $"printf a | printf b | cat" |> shell.capture(sh) {
        Ok(_) -> msgs.push("ok")
        Err(f) -> msgs.push(f.message())
    }
}
let after = countFds() || -1
print "before=${before} after=${after}"
for m in msgs {
    print "msg=${m}"
}
`

func buildShellFdSafetyProg(t *testing.T, src string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"fdsafety\"\n    kigumi: \"0.1\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.kg"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "prog")
	var buildErr bytes.Buffer
	ok, err := testBuild(m, exe, &buildErr)
	if err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}
	return exe
}

// TestShellRedirectFailureLeavesNoFd: a redirect
// that can never open must fail every stage cleanly and the process's own
// fd count (read back from /proc/self/fd) must return to where it started,
// across repeated failures.
func TestShellRedirectFailureLeavesNoFd(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("uses /proc/self/fd")
	}
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	exe := buildShellFdSafetyProg(t, shellFdSafetyBadRedirect)
	out, err := exec.Command(exe).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	if !strings.HasPrefix(got, "errs=5 before=") {
		t.Fatalf("expected all 5 stages to fail cleanly, got %q", got)
	}
	before, after := parseBeforeAfter(t, got)
	if before != after {
		t.Errorf("fd count grew across 5 failed redirects: before=%d after=%d (%q)", before, after, got)
	}
}

// TestShellDevnullOpenFailure: a low RLIMIT_NOFILE (applied
// to the child only, via `ulimit -n` in a bash wrapper) makes /dev/null's
// open() fail, and it must be reported instead of dup2'ing -1 onto stdin.
// Repeating the run under the same tight limit must fail with the exact
// same message each time: a diverging message would mean an earlier run
// leaked descriptors and starved the next one.
func TestShellDevnullOpenFailure(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("uses /proc/self/fd and a bash ulimit wrapper")
	}
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	exe := buildShellFdSafetyProg(t, shellFdSafetyLowNofile)
	out, err := exec.Command("bash", "-c", `ulimit -n 9; exec "$0"`, exe).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 6 {
		t.Fatalf("expected a before/after line and 5 message lines, got %q", out)
	}
	before, after := parseBeforeAfter(t, lines[0])
	if before != after {
		t.Errorf("fd count grew across 5 runs under RLIMIT_NOFILE=9: before=%d after=%d (%q)", before, after, out)
	}
	// Which syscall hits EMFILE first depends on how many descriptors the
	// child inherited (a parallel `go test ./...` leaves more open than a
	// lone run), so either the /dev/null open or the pipe may be the one.
	wants := map[string]bool{
		"msg=cannot open /dev/null: Too many open files": true,
		"msg=pipe: Too many open files":                  true,
	}
	if !wants[lines[1]] {
		t.Fatalf("expected an EMFILE report from open or pipe, got %q in %q", lines[1], out)
	}
	for _, line := range lines[2:] {
		if line != lines[1] {
			t.Errorf("expected every run to fail the same way, got %q (want %q) in %q", line, lines[1], out)
		}
	}
}

// TestShellPipeSetupFailureLeavesNoFd covers a similar fd-safety class one
// step earlier: a 3-stage pipeline needs 3 pipe pairs (18 fds)
// before a single fork() runs, so a tight RLIMIT_NOFILE makes one of those
// pipe() calls fail mid-setup. The pipes already made for earlier stages
// (and earlier pipes of the same stage) must not leak just because the
// setup loop bails out through `?` before any child exists to inherit them.
func TestShellPipeSetupFailureLeavesNoFd(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("uses /proc/self/fd and a bash ulimit wrapper")
	}
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	exe := buildShellFdSafetyProg(t, shellFdSafetyPipeSetup)
	out, err := exec.Command("bash", "-c", `ulimit -n 18; exec "$0"`, exe).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 6 {
		t.Fatalf("expected a before/after line and 5 message lines, got %q", out)
	}
	before, after := parseBeforeAfter(t, lines[0])
	if before != after {
		t.Errorf("fd count grew across 5 runs under RLIMIT_NOFILE=18: before=%d after=%d (%q)", before, after, out)
	}
	want := "msg=pipe: Too many open files"
	for _, line := range lines[1:] {
		if line != want {
			t.Errorf("expected every run to fail the same way, got %q (want %q) in %q", line, want, out)
		}
	}
}

func parseBeforeAfter(t *testing.T, line string) (before, after int) {
	t.Helper()
	var skip int
	if n, err := fmt.Sscanf(line, "errs=%d before=%d after=%d", &skip, &before, &after); err == nil && n == 3 {
		return
	}
	if _, err := fmt.Sscanf(line, "before=%d after=%d", &before, &after); err != nil {
		t.Fatalf("cannot parse fd counts out of %q: %v", line, err)
	}
	return
}
