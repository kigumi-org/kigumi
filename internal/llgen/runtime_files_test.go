package llgen

import "testing"

// TestRuntimeFilesSignalGating checks rt_signal.c drops out for every OS
// without POSIX <signal.h>, not just for "--sys none"/freestanding.
func TestRuntimeFilesSignalGating(t *testing.T) {
	has := func(files []RuntimeFile, name string) bool {
		for _, f := range files {
			if f.Name == name {
				return true
			}
		}
		return false
	}
	cases := []struct {
		sys, os string
		want    bool
	}{
		{"posix", "linux", true},
		{"posix", "darwin", true},
		{"posix", "freebsd", true},
		{"posix", "netbsd", true},
		{"posix", "openbsd", true},
		{"posix", "wasi", false},
		{"posix", "windows", false},
		{"none", "linux", false},
		{"none", "freestanding", false},
	}
	for _, c := range cases {
		got := has(RuntimeFiles(c.sys, c.os), "rt_signal.c")
		if got != c.want {
			t.Errorf("RuntimeFiles(%q, %q) has rt_signal.c = %v, want %v", c.sys, c.os, got, c.want)
		}
	}
}
