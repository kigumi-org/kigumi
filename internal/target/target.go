// Package target describes what the compiler builds for: the pointer width
// that fixes usize and isize and the names the platform
// files use. It is immutable and passed explicitly; nothing reads it from
// the environment.
package target

import "runtime"

// Layout is the width-dependent part of a target.
type Layout struct {
	OS, Arch string
	// PtrBits is the pointer width: the width of usize, isize and raw pointers.
	PtrBits int
}

var archBits = map[string]int{
	"amd64": 64, "arm64": 64, "riscv64": 64, "386": 32, "arm": 32, "wasm32": 32,
	"mips": 32, "mipsle": 32, "mips64": 64, "mips64le": 64,
	"ppc64": 64, "ppc64le": 64, "s390x": 64, "loong64": 64,
}

// ForArch gives the layout of an OS / architecture pair. It panics for an
// architecture kigumi doesn't know the pointer width of; callers reach
// this only through Host(), since ParseTarget rejects an unknown
// --target arch before it gets here.
func ForArch(os, arch string) Layout {
	bits, ok := archBits[arch]
	if !ok {
		panic("target: unknown architecture " + arch)
	}
	return Layout{OS: os, Arch: arch, PtrBits: bits}
}

// Host is the layout of the machine kigumi runs on.
func Host() Layout {
	arch := runtime.GOARCH
	if arch == "wasm" {
		arch = "wasm32"
	}
	return ForArch(runtime.GOOS, arch)
}
