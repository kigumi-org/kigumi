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

const filesListSrc = `import fs from std/fs

let files = host.files()
let dirPath = fs.Path.fromString("sub")?
match files.list(dirPath) {
    Ok(names) -> print "count=${names.len()}"
    Err(e) -> print "err: ${e.message()}"
}
`

// failReaddirC overrides libc's readdir(3): the call that would return the
// directory's 3rd entry instead fails with EBADF, simulating a genuine
// mid-stream readdir() error (errno-reporting-4) rather than end-of-stream.
// It is linked into the test binary via KIGUMI_CFLAGS, so it takes priority
// over libc's own readdir at link time -- no LD_PRELOAD needed.
const failReaddirC = `#define _GNU_SOURCE
#include <dirent.h>
#include <errno.h>
#include <dlfcn.h>

static int kg_test_readdir_calls = 0;

struct dirent *readdir(DIR *d) {
    static struct dirent *(*real_readdir)(DIR *) = 0;
    if (!real_readdir) real_readdir = dlsym(RTLD_NEXT, "readdir");
    kg_test_readdir_calls++;
    if (kg_test_readdir_calls == 3) {
        errno = 9;
        return 0;
    }
    return real_readdir(d);
}
`

// failReaddirAndClosedirC is like failReaddirC but also forces closedir(3)
// to fail on every call, so a naive fix that reads errno after closedir
// would report closedir's error instead of readdir's real one.
const failReaddirAndClosedirC = `#define _GNU_SOURCE
#include <dirent.h>
#include <errno.h>
#include <dlfcn.h>

static int kg_test_readdir_calls = 0;

struct dirent *readdir(DIR *d) {
    static struct dirent *(*real_readdir)(DIR *) = 0;
    if (!real_readdir) real_readdir = dlsym(RTLD_NEXT, "readdir");
    kg_test_readdir_calls++;
    if (kg_test_readdir_calls == 3) {
        errno = 9;
        return 0;
    }
    return real_readdir(d);
}

int closedir(DIR *d) {
    errno = 5;
    return -1;
}
`

// failReaddirEintrC is like failReaddirC but fails the 3rd call with EINTR
// (errno 4, a signal interrupting the underlying getdents(2) with no
// SA_RESTART) instead of EBADF: a transient condition every other
// read/write retry loop in this codebase retries rather than surfaces.
const failReaddirEintrC = `#define _GNU_SOURCE
#include <dirent.h>
#include <errno.h>
#include <dlfcn.h>

static int kg_test_readdir_calls = 0;

struct dirent *readdir(DIR *d) {
    static struct dirent *(*real_readdir)(DIR *) = 0;
    if (!real_readdir) real_readdir = dlsym(RTLD_NEXT, "readdir");
    kg_test_readdir_calls++;
    if (kg_test_readdir_calls == 3) {
        errno = 4;
        return 0;
    }
    return real_readdir(d);
}
`

// TestFilesListReaddirError checks that Files.list distinguishes a
// mid-iteration readdir(3) failure from a clean end-of-directory
// (errno-reporting-4): a genuine error must surface as Err, not be folded
// into a truncated Ok. VM and interp back Files.list with Go's os.ReadDir,
// which already reports such an error, so only native needs the fix; the
// interp/vm subtests are a baseline sanity check for the (unforced) happy
// path shared with the fixture.
func TestFilesListReaddirError(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "main", "sub"), 0o755)
	for _, name := range []string{"a", "b", "c"} {
		os.WriteFile(filepath.Join(root, "main", "sub", name), nil, 0o644)
	}
	os.WriteFile(filepath.Join(root, "main/main.kg"), []byte(filesListSrc), 0o644)

	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Entry: "main/main.kg"})
	if err != nil {
		t.Fatal(err)
	}

	for _, engine := range []string{"vm", "interp"} {
		t.Run(engine, func(t *testing.T) {
			t.Chdir(filepath.Join(root, "main"))
			var out, errOut bytes.Buffer
			code, err := driver.RunWith(m, driver.RunOptions{Engine: engine}, &out, &errOut, []string{"main/main.kg"})
			if err != nil || code != 0 {
				t.Fatalf("%s: err %v exit %d\n%s", engine, err, code, errOut.String())
			}
			if got := strings.TrimSpace(out.String()); got != "count=3" {
				t.Errorf("%s stdout = %q, want %q", engine, got, "count=3")
			}
		})
	}

	t.Run("native", func(t *testing.T) {
		if driver.CCompiler() == nil {
			t.Skip("no C compiler")
		}

		t.Run("baseline", func(t *testing.T) {
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
			if got := strings.TrimSpace(out.String()); got != "count=3" {
				t.Errorf("stdout = %q, want %q", got, "count=3")
			}
		})

		t.Run("mid_stream_failure", func(t *testing.T) {
			helper := filepath.Join(t.TempDir(), "fail_readdir.c")
			if err := os.WriteFile(helper, []byte(failReaddirC), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("KIGUMI_CFLAGS", helper+" -ldl")
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
			got := strings.TrimSpace(out.String())
			if !strings.HasPrefix(got, "err:") {
				t.Errorf("stdout = %q, want an Err (a forced readdir() failure must not look like a clean, truncated Ok)", got)
			}
		})

		t.Run("mid_stream_eintr_retried", func(t *testing.T) {
			helper := filepath.Join(t.TempDir(), "fail_readdir_eintr.c")
			if err := os.WriteFile(helper, []byte(failReaddirEintrC), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("KIGUMI_CFLAGS", helper+" -ldl")
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
			if got := strings.TrimSpace(out.String()); got != "count=3" {
				t.Errorf("stdout = %q, want %q (EINTR must be retried, not surfaced as a failure)", got, "count=3")
			}
		})

		// The reported error must be readdir's (EBADF, "Bad file descriptor"),
		// not closedir's (EIO, "Input/output error"), even though closedir
		// runs after.
		t.Run("mid_stream_failure_closedir_also_fails", func(t *testing.T) {
			helper := filepath.Join(t.TempDir(), "fail_readdir_closedir.c")
			if err := os.WriteFile(helper, []byte(failReaddirAndClosedirC), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("KIGUMI_CFLAGS", helper+" -ldl")
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
			got := strings.TrimSpace(out.String())
			if !strings.Contains(got, "Bad file descriptor") {
				t.Errorf("stdout = %q, want the readdir() error (EBADF, Bad file descriptor), not closedir's (EIO, Input/output error)", got)
			}
		})
	})
}
