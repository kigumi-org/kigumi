package driver

import (
	"strings"
	"testing"
)

func TestFirstDiffOffset(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		a, b       string
		wantOffset int
		wantEqual  bool
	}{
		{"identical", "abcdef", "abcdef", 0, true},
		{"empty both", "", "", 0, true},
		{"differ mid", "abcXef", "abcYef", 3, false},
		{"differ first byte", "Xbc", "Ybc", 0, false},
		{"b is prefix of a", "abcdef", "abc", 3, false},
		{"a is prefix of b", "abc", "abcdef", 3, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			offset, equal := firstDiffOffset([]byte(c.a), []byte(c.b))
			if offset != c.wantOffset || equal != c.wantEqual {
				t.Errorf("firstDiffOffset(%q, %q) = (%d, %v), want (%d, %v)", c.a, c.b, offset, equal, c.wantOffset, c.wantEqual)
			}
		})
	}
}

// TestIsolatedCacheEnvOverridesOnce covers the duplicate-key hazard a plain
// append(os.Environ(), "K=v") has: two builds must each see exactly one
// ZIG_*_CACHE_DIR value, pinned under their own dir, even if the ambient
// environment (or a previous call) already set one.
func TestIsolatedCacheEnvOverridesOnce(t *testing.T) {
	t.Parallel()
	base := []string{"PATH=/usr/bin", "ZIG_GLOBAL_CACHE_DIR=/ambient/global", "ZIG_LOCAL_CACHE_DIR=/ambient/local"}
	env := isolatedCacheEnv("/tmp/kigumi-repro-a", base)
	count := func(key string) int {
		n := 0
		for _, kv := range env {
			if strings.HasPrefix(kv, key+"=") {
				n++
			}
		}
		return n
	}
	if n := count("ZIG_GLOBAL_CACHE_DIR"); n != 1 {
		t.Errorf("ZIG_GLOBAL_CACHE_DIR appears %d times, want 1", n)
	}
	if n := count("ZIG_LOCAL_CACHE_DIR"); n != 1 {
		t.Errorf("ZIG_LOCAL_CACHE_DIR appears %d times, want 1", n)
	}
	if n := count("PATH"); n != 1 {
		t.Errorf("PATH appears %d times, want 1", n)
	}
	other := isolatedCacheEnv("/tmp/kigumi-repro-b", base)
	for _, kv := range env {
		if strings.HasPrefix(kv, "ZIG_GLOBAL_CACHE_DIR=") {
			for _, kv2 := range other {
				if kv == kv2 {
					t.Errorf("two calls with different dirs produced the same %s", kv)
				}
			}
		}
	}
}
