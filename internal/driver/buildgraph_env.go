package driver

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kigumi/internal/buildgraph"
)

// zigCacheDir is where the built-in toolchain Framework's zig-backed tools
// cache their work: under the build's own output directory, the last of
// Graph.Roots, rather than a host directory the
// tool env allowlist otherwise hides from them (there is no HOME).
func zigCacheDir(g *buildgraph.Graph) string {
	if len(g.Roots) == 0 {
		return ""
	}
	return filepath.Join(g.Roots[len(g.Roots)-1], ".kigumi-zig-cache")
}

func placeholderIndex(a string, n int) (int, error) {
	i, err := strconv.Atoi(a[strings.Index(a, ":")+1:])
	if err != nil || i < 0 || i >= n {
		return 0, fmt.Errorf("placeholder %s is malformed or out of range (%d declared)", a, n)
	}
	return i, nil
}

// withinSandbox mirrors the interpreter's check for the executing side:
// symlinks are followed on both the path and the roots.
func withinSandbox(roots []string, path string) bool {
	resolve := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return filepath.Clean(p)
	}
	path = resolve(path)
	for _, r := range roots {
		rel, err := filepath.Rel(resolve(r), path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// toolEnv is the allowlisted environment plus the target as explicit
// input; an empty allowlist still yields a defined, empty environment.
func toolEnv(allow []string, target buildgraph.Target) []string {
	env := append([]string{}, allow...)
	return append(env, "KIGUMI_TARGET_TRIPLE="+target.Triple, "KIGUMI_TARGET_SYS="+target.Sys)
}

func envNames(env []string) string {
	names := make([]string, 0, len(env))
	for _, kv := range env {
		names = append(names, strings.SplitN(kv, "=", 2)[0])
	}
	return strings.Join(names, ",")
}

// checkOutputSlot refuses a pre-existing symlink, directory or special
// file at path: writing through a symlink there could land outside the
// output directory, and the other kinds are not something a build step
// should silently replace. A plain file or nothing at path is fine.
func checkOutputSlot(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("output %s already exists as a symlink", path)
	case fi.IsDir():
		return fmt.Errorf("output %s already exists as a directory", path)
	case !fi.Mode().IsRegular():
		return fmt.Errorf("output %s already exists as a special file", path)
	}
	return nil
}

// writeGraphOutput writes content to path once checkOutputSlot passes; the
// atomic rename replaces a symlink planted at path instead of following it.
// Unlike writeFileAtomic's default, perm is enforced even when path already
// exists: a build-graph output is a deterministic function of the graph, not
// user source to preserve the mode of (a stale or tampered --out-dir must
// not leak its old mode into freshly generated content).
func writeGraphOutput(path string, content []byte, perm os.FileMode) error {
	if err := checkOutputSlot(path); err != nil {
		return err
	}
	if err := writeFileAtomic(path, content, perm); err != nil {
		return err
	}
	return os.Chmod(path, perm)
}
