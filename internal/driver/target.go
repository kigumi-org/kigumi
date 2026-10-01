package driver

import (
	"fmt"
	"runtime"
	"strings"

	"kigumi/internal/target"
)

// Target is what `kigumi build --target` compiles for: a zig-style triple
// (`x86_64-linux-musl`, `aarch64-linux-gnu`, `x86_64-freestanding`). OS and
// Arch use the platform-file names; Sys picks the runtime layer.
type Target struct {
	Triple string
	OS     string
	Arch   string
	// Sys is "posix" or "none"; a freestanding OS forces "none".
	Sys string
	// Allocator is "" (the general allocator) or "none": the build has no
	// default allocator and the checker enforces that from the entry.
	Allocator string
}

var targetArch = map[string]string{"x86_64": "amd64", "aarch64": "arm64", "riscv64": "riscv64", "wasm32": "wasm32", "x86": "386", "arm": "arm"}

var targetOS = map[string]string{"linux": "linux", "macos": "darwin", "windows": "windows", "freebsd": "freebsd", "netbsd": "netbsd", "openbsd": "openbsd", "wasi": "wasi", "freestanding": "freestanding", "uefi": "uefi", "none": "freestanding"}

// HostTarget is the machine kigumi runs on.
func HostTarget() Target {
	return Target{OS: runtime.GOOS, Arch: runtime.GOARCH, Sys: "posix"}
}

// ParseTarget reads a triple; "" is the host.
func ParseTarget(triple, sys string) (Target, error) {
	t := HostTarget()
	if triple != "" {
		parts := strings.Split(triple, "-")
		if len(parts) < 2 {
			return t, fmt.Errorf("target %q: want <arch>-<os>[-<abi>]", triple)
		}
		arch, ok := targetArch[parts[0]]
		if !ok {
			return t, fmt.Errorf("target %q: unknown architecture %s", triple, parts[0])
		}
		os, ok := targetOS[parts[1]]
		if !ok {
			return t, fmt.Errorf("target %q: unknown OS %s", triple, parts[1])
		}
		t = Target{Triple: triple, OS: os, Arch: arch, Sys: "posix"}
	}
	if t.Freestanding() {
		t.Sys = "none"
	}
	if sys != "" {
		if sys != "posix" && sys != "none" {
			return t, fmt.Errorf("sys %q: want posix or none", sys)
		}
		t.Sys = sys
	}
	return t, nil
}

// Freestanding targets have no operating system: the build is an archive
// with a `kigumi_main` entry for the embedder's startup code.
func (t Target) Freestanding() bool { return t.OS == "freestanding" || t.OS == "uefi" }

// Bare targets get Freestanding's archive output and `_bare.kg` files even
// on a hosted OS, since `--sys none` also means no OS services
// compileTriple and Layout stay keyed on
// Freestanding alone: a host-OS bare archive still uses the real OS
// headers and ABI, not the musl substitution.
func (t Target) Bare() bool { return t.Freestanding() || t.Sys == "none" }

// Layout is the width-dependent view of the target for the checker,
// the interpreter and the backend.
func (t Target) Layout() target.Layout { return target.ForArch(t.OS, t.Arch) }

// NoDefaultAllocator reports the `--allocator none` profile.
func (t Target) NoDefaultAllocator() bool { return t.Allocator == "none" }

// compileTriple is what the C compiler sees. A freestanding target borrows
// the arch's musl headers so the runtime's libc calls type-check; the
// symbols themselves come from the embedder.
func (t Target) compileTriple() string {
	if t.Triple == "" {
		return ""
	}
	if t.Freestanding() {
		return strings.Split(t.Triple, "-")[0] + "-linux-musl"
	}
	return t.Triple
}
