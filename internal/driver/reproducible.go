package driver

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Isolating the compiler cache per build makes the second build a real
// compile rather than a cache hit that would pass vacuously, for the
// --reproducible flag. Neither build writes to out directly, so a
// mismatch leaves whatever was already there untouched.
func buildReproducible(m *Module, out string, stderr io.Writer, opts BuildOptions) (bool, error) {
	name := filepath.Base(out)
	var data [2][]byte
	var mode os.FileMode
	for i := range data {
		dir, err := os.MkdirTemp("", "kigumi-repro")
		if err != nil {
			return false, err
		}
		defer os.RemoveAll(dir)
		buildOut := filepath.Join(dir, name)
		once := opts
		once.Reproducible = false
		once.pinCompDir = true
		once.ccEnv = isolatedCacheEnv(dir, opts.Env)
		ok, err := buildOnce(m, buildOut, stderr, once)
		if err != nil || !ok {
			return ok, err
		}
		st, err := os.Stat(buildOut)
		if err != nil {
			return false, err
		}
		if i == 0 {
			mode = st.Mode()
		}
		data[i], err = os.ReadFile(buildOut)
		if err != nil {
			return false, err
		}
	}
	if off, equal := firstDiffOffset(data[0], data[1]); !equal {
		return false, fmt.Errorf("build is not reproducible: %s differs at byte offset %d (%d vs %d bytes)", name, off, len(data[0]), len(data[1]))
	}
	if err := writeGraphOutput(out, data[0], mode); err != nil {
		return false, err
	}
	if err := os.Chmod(out, mode); err != nil {
		return false, err
	}
	return true, nil
}

// firstDiffOffset is the offset of the first byte at which a and b differ;
// when one is a prefix of the other, that is the shorter one's length.
func firstDiffOffset(a, b []byte) (offset int, equal bool) {
	n := min(len(a), len(b))
	for i := range n {
		if a[i] != b[i] {
			return i, false
		}
	}
	if len(a) != len(b) {
		return n, false
	}
	return 0, true
}

// internal/driver may not read the ambient environment itself,
// so base comes from the caller rather than a direct process-environment
// read here.
func isolatedCacheEnv(dir string, base []string) []string {
	env := make([]string, 0, len(base)+2)
	for _, kv := range base {
		if strings.HasPrefix(kv, "ZIG_GLOBAL_CACHE_DIR=") || strings.HasPrefix(kv, "ZIG_LOCAL_CACHE_DIR=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"ZIG_GLOBAL_CACHE_DIR="+filepath.Join(dir, "zig-cache-global"),
		"ZIG_LOCAL_CACHE_DIR="+filepath.Join(dir, "zig-cache-local"))
}
