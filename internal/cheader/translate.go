package cheader

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Runner executes `zig translate-c` and returns its stdout; it exists so
// tests can substitute a counting fake instead of the real zig binary
// (cache_test.go: a cache hit must not invoke it at all).
type Runner func(zigPath, target, headerPath string) ([]byte, error)

// RunZig is the default Runner: `zig translate-c -lc [-target T] header.h`.
// `-lc` makes translate-c look at the target's bundled-sysroot libc headers
// at all (without it even `#include <string.h>` fails to resolve); the
// same sysroot already covers freestanding includes, matching
// BuildWith's `zig cc` (target.go's compileTriple).
func RunZig(zigPath, target, headerPath string) ([]byte, error) {
	args := []string{"translate-c", "-lc"}
	if target != "" {
		args = append(args, "-target", target)
	}
	args = append(args, headerPath)
	cmd := exec.Command(zigPath, args...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("zig translate-c %s: %w\n%s", headerPath, err, errOut.String())
	}
	return out.Bytes(), nil
}

// zigFingerprint identifies the zig binary at zigPath without running it
// (its mtime and size), so a cache hit costs one stat call, never a zig
// invocation; a rebuilt or upgraded zig gets a new fingerprint and
// regenerates (cache.go's cacheKey: `zig version` itself was
// rejected for this precisely because a hit must invoke zig zero times).
func zigFingerprint(zigPath string) (string, error) {
	st, err := os.Stat(zigPath)
	if err != nil {
		return "", fmt.Errorf("stat zig at %s: %w", zigPath, err)
	}
	return fmt.Sprintf("%d-%d", st.Size(), st.ModTime().UnixNano()), nil
}

// translateBoth runs translate-c on header and, for the baseline Generate
// diffs against, on an empty file with the same flags.
func translateBoth(run Runner, zigPath, target, header string) (full, baseline string, err error) {
	fullOut, err := run(zigPath, target, header)
	if err != nil {
		return "", "", err
	}
	dir, err := os.MkdirTemp("", "kigumi-cheader")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(dir)
	empty := filepath.Join(dir, "empty.h")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		return "", "", err
	}
	baseOut, err := run(zigPath, target, empty)
	if err != nil {
		return "", "", err
	}
	return string(fullOut), string(baseOut), nil
}
