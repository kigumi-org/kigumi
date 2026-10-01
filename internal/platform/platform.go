// Package platform names the operating systems and architectures a file
// name can select with a suffix: main_linux.kg loads on Linux only,
// util_arm64.kg on arm64 only, x_linux_arm64.kg on both. `hosted` and
// `bare` are pseudo names for any OS and for no OS.
package platform

import (
	"path/filepath"
	"runtime"
	"strings"
)

var OS = map[string]bool{
	"linux": true, "darwin": true, "windows": true, "freebsd": true, "openbsd": true, "netbsd": true, "wasi": true,
	"freestanding": true, "uefi": true,
}

var Arch = map[string]bool{
	"amd64": true, "arm64": true, "riscv64": true, "wasm32": true, "386": true, "arm": true,
}

// Bare reports a target without an operating system.
func Bare(os string) bool { return os == "freestanding" || os == "uefi" }

// Host is the running machine as a target.
func Host() (os, arch string) {
	arch = runtime.GOARCH
	if arch == "wasm" {
		arch = "wasm32"
	}
	return runtime.GOOS, arch
}

// File reports whether a file name carries a platform suffix and, if so,
// whether that suffix matches the target. The extension is ignored so the
// rule serves .kg, .c and .s alike.
func File(name, os, arch string) (tagged, matches bool) {
	base := strings.TrimSuffix(strings.TrimSuffix(name, filepath.Ext(name)), "_test")
	parts := strings.Split(base, "_")
	if len(parts) < 2 {
		return false, false
	}
	last := parts[len(parts)-1]
	switch {
	case Arch[last]:
		if len(parts) >= 3 && OS[parts[len(parts)-2]] {
			return true, parts[len(parts)-2] == os && last == arch
		}
		return true, last == arch
	case OS[last]:
		return true, last == os
	case last == "hosted":
		return true, !Bare(os)
	case last == "bare":
		return true, Bare(os)
	}
	return false, false
}
