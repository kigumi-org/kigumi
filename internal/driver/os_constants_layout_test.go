package driver_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// TestSockoptConstants pins std/net/sockopt_<os>.kg's per-platform integer
// constants against the headers the driver's own zig toolchain ships
// (libc/include/<target>/...).
func TestSockoptConstants(t *testing.T) {
	type row struct {
		fn   string
		want int64
	}
	// Values and their header source, verified against this repo's own
	// zig toolchain.
	cases := map[string][]row{
		// any-darwin-any/sys/socket.h, sys/fcntl.h, sys/errno.h.
		"darwin": {
			{"solSocket", 0xffff}, {"soReuseAddr", 4},
			{"soRcvTimeo", 0x1006}, {"soSndTimeo", 0x1005},
			{"oNonBlock", 4}, {"pollOut", 4}, {"soError", 0x1007}, {"eintr", 4},
		},
		// generic-freebsd/sys/socket.h, fcntl.h, errno.h.
		"freebsd": {
			{"solSocket", 0xffff}, {"soReuseAddr", 4},
			{"soRcvTimeo", 0x1006}, {"soSndTimeo", 0x1005},
			{"oNonBlock", 4}, {"pollOut", 4}, {"soError", 0x1007}, {"eintr", 4},
		},
		// generic-musl/bits/socket.h, bits/fcntl.h, bits/errno.h.
		"linux": {
			{"solSocket", 1}, {"soReuseAddr", 2},
			{"soRcvTimeo", 20}, {"soSndTimeo", 21},
			{"oNonBlock", 2048}, {"pollOut", 4}, {"soError", 4}, {"eintr", 4},
		},
		// generic-netbsd/sys/socket.h: SO_RCVTIMEO/SO_SNDTIMEO were
		// renumbered to 0x100c/0x100b; 0x1006/0x1005 are the retired
		// SO_ORCVTIMEO/SO_OSNDTIMEO, commented out in the real header.
		"netbsd": {
			{"solSocket", 0xffff}, {"soReuseAddr", 4},
			{"soRcvTimeo", 0x100c}, {"soSndTimeo", 0x100b},
			{"oNonBlock", 4}, {"pollOut", 4}, {"soError", 0x1007}, {"eintr", 4},
		},
		// generic-openbsd/sys/socket.h, fcntl.h, errno.h.
		"openbsd": {
			{"solSocket", 0xffff}, {"soReuseAddr", 4},
			{"soRcvTimeo", 0x1006}, {"soSndTimeo", 0x1005},
			{"oNonBlock", 4}, {"pollOut", 4}, {"soError", 0x1007}, {"eintr", 4},
		},
		// wasm-wasi-musl/__header_sys_socket.h, __header_fcntl.h,
		// __header_poll.h, wasi/api.h: solSocket/oNonBlock/pollOut/eintr
		// have real wasi-libc values; soReuseAddr/soRcvTimeo/soSndTimeo/
		// soError are undefined outside the wasip2 ABI this driver never
		// targets, so these four keep musl's numbers as placeholders
		// (see the doc comments in sockopt_wasi.kg).
		"wasi": {
			{"solSocket", 0x7fffffff}, {"soReuseAddr", 2},
			{"soRcvTimeo", 20}, {"soSndTimeo", 21},
			{"oNonBlock", 4}, {"pollOut", 2}, {"soError", 4}, {"eintr", 27},
		},
		// any-windows-any/{winsock2.h,errno.h}: fcntl/poll aren't real
		// Winsock operations, kept only so extern(C) calls type-check.
		"windows": {
			{"solSocket", 0xffff}, {"soReuseAddr", 4},
			{"soRcvTimeo", 0x1006}, {"soSndTimeo", 0x1005},
			{"oNonBlock", 4}, {"pollOut", 0x10}, {"soError", 0x1007}, {"eintr", 4},
		},
	}

	for os, rows := range cases {
		os, rows := os, rows
		t.Run(os, func(t *testing.T) {
			src := platformFile(t, "net", "sockopt_"+os+".kg")
			for _, r := range rows {
				got, ok := fnLiteralInt(src, r.fn)
				if !ok {
					t.Errorf("no `fn %s() -> ... { <literal> }` found", r.fn)
					continue
				}
				if got != r.want {
					t.Errorf("%s() = %#x, want %#x", r.fn, got, r.want)
				}
			}
		})
	}
}

// TestShellOpenFlagsConstants pins std/shell/flags_<os>.kg's OpenFlags
// against the same headers.
func TestShellOpenFlagsConstants(t *testing.T) {
	type want struct{ creat, trunc, append, wronly int64 }
	cases := map[string]want{
		"darwin":  {512, 1024, 8, 1},
		"freebsd": {512, 1024, 8, 1},
		"linux":   {64, 512, 1024, 1},
		"netbsd":  {512, 1024, 8, 1},
		"openbsd": {512, 1024, 8, 1},
		// __WASI_OFLAGS_CREAT/TRUNC << 12, __WASI_FDFLAGS_APPEND, and the
		// raw O_WRONLY bit (0x10000000) from __header_fcntl.h.
		"wasi": {4096, 32768, 1, 0x10000000},
		// mingw-w64 fcntl.h: _O_CREAT 0x100, _O_TRUNC 0x200, _O_APPEND
		// 0x8, _O_WRONLY 0x1.
		"windows": {256, 512, 8, 1},
	}

	for os, w := range cases {
		os, w := os, w
		t.Run(os, func(t *testing.T) {
			src := platformFile(t, "shell", "flags_"+os+".kg")
			fields := map[string]int64{"creat": w.creat, "trunc": w.trunc, "append": w.append, "wronly": w.wronly}
			for name, want := range fields {
				got, ok := recordFieldInt(src, name)
				if !ok {
					t.Errorf("no %q field found in openFlags()'s OpenFlags literal", name)
					continue
				}
				if got != want {
					t.Errorf("%s = %#x, want %#x", name, got, want)
				}
			}
		})
	}
}

// TestOpenbsdAddrInfoLayout pins std/net/addrinfo_openbsd.kg's field order
// against OpenBSD's real struct addrinfo (zig ships this header at
// libc/include/generic-openbsd/netdb.h), the reverse of Darwin/FreeBSD/
// NetBSD/Windows, which all put ai_canonname before ai_addr:
//
//	struct addrinfo {
//		int ai_flags;
//		int ai_family;
//		int ai_socktype;
//		int ai_protocol;
//		socklen_t ai_addrlen;
//		struct sockaddr *ai_addr;
//		char *ai_canonname;
//		struct addrinfo *ai_next;
//	};
func TestOpenbsdAddrInfoLayout(t *testing.T) {
	src := platformFile(t, "net", "addrinfo_openbsd.kg")
	fields := parseLayoutCFields(t, src, "AddrInfo")
	wantOrder := []string{"flags", "family", "socktype", "protocol", "addrlen", "addr", "canonname", "next"}
	var gotOrder []string
	for _, f := range fields {
		gotOrder = append(gotOrder, f.name)
	}
	if strings.Join(gotOrder, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("field order = %v, want %v (OpenBSD declares ai_addr before ai_canonname)", gotOrder, wantOrder)
	}

	offsets := layoutCOffsets(fields, windowsAmd64Size)
	wantOffsets := map[string]int64{
		"flags": 0, "family": 4, "socktype": 8, "protocol": 12,
		"addrlen": 16, "addr": 24, "canonname": 32, "next": 40,
	}
	for name, want := range wantOffsets {
		if got := offsets[name]; got != want {
			t.Errorf("offset of %s = %d, want %d", name, got, want)
		}
	}
}

// TestFsOsShellErrnoConstants pins std/fs, std/os, std/shell and std/entropy's
// per-OS errno_<os>.kg constants (eexist/eintr) against the same zig-shipped
// headers.
func TestFsOsShellErrnoConstants(t *testing.T) {
	// generic-{freebsd,netbsd,openbsd}/sys/errno.h, any-darwin-any/sys/
	// errno.h, any-windows-any/errno.h, generic-musl/bits/errno.h: EINTR=4,
	// EEXIST=17 on every one of these. wasm-wasi-musl/__errno_values.h maps
	// EINTR to __WASI_ERRNO_INTR=27 and EEXIST to __WASI_ERRNO_EXIST=20
	// (wasi/api.h), not musl's numbers wasi-libc's top half otherwise
	// shares with Linux.
	eintr := map[string]int64{
		"darwin": 4, "freebsd": 4, "linux": 4, "netbsd": 4, "openbsd": 4, "windows": 4,
		"wasi": 27,
	}
	eexist := map[string]int64{
		"darwin": 17, "freebsd": 17, "linux": 17, "netbsd": 17, "openbsd": 17, "windows": 17,
		"wasi": 20,
	}

	for os, want := range eintr {
		os, want := os, want
		t.Run("os_eintr_"+os, func(t *testing.T) {
			checkFnLiteral(t, "os", "errno_"+os+".kg", "eintr", want)
		})
		t.Run("shell_eintr_"+os, func(t *testing.T) {
			checkFnLiteral(t, "shell", "errno_"+os+".kg", "eintr", want)
		})
		t.Run("fs_eintr_"+os, func(t *testing.T) {
			checkFnLiteral(t, "fs", "errno_"+os+".kg", "eintr", want)
		})
		t.Run("entropy_eintr_"+os, func(t *testing.T) {
			checkFnLiteral(t, "entropy", "errno_"+os+".kg", "eintr", want)
		})
	}
	for os, want := range eexist {
		os, want := os, want
		t.Run("fs_eexist_"+os, func(t *testing.T) {
			checkFnLiteral(t, "fs", "errno_"+os+".kg", "eexist", want)
		})
	}
}

func checkFnLiteral(t *testing.T, pkg, file, fn string, want int64) {
	t.Helper()
	src := platformFile(t, pkg, file)
	got, ok := fnLiteralInt(src, fn)
	if !ok {
		t.Fatalf("no `fn %s() -> ... { <literal> }` found in %s/%s", fn, pkg, file)
	}
	if got != want {
		t.Errorf("%s/%s: %s() = %d, want %d", pkg, file, fn, got, want)
	}
}

// TestWasi32PlatformFilesCheck compiles std/net, std/shell, std/entropy,
// std/fs and std/os for wasm32-wasi with the checker (no linker needed), so
// a layout(C) or arity mistake in a wasi platform file is a normal compile
// error rather than something only a real wasm32-wasi run would surface.
func TestWasi32PlatformFilesCheck(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"wasichk\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("import net from std/net\nimport shell from std/shell\nimport entropy from std/entropy\nimport fs from std/fs\nimport os from std/os\nprint \"ok\"\n"), 0o644)
	std, err := filepath.Abs("../../std")
	if err != nil {
		t.Fatal(err)
	}
	target, err := driver.ParseTarget("wasm32-wasi", "")
	if err != nil {
		t.Fatal(err)
	}
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.HasErrors() {
		t.Fatalf("wasm32-wasi checking failed:\n%s", res.Render())
	}
}

func platformFile(t *testing.T, pkg, name string) string {
	t.Helper()
	std, err := filepath.Abs("../../std")
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(std, pkg, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}

// fnLiteralInt extracts N from `fn <name>() -> <type> { N }`, accepting
// decimal or 0x-hex.
func fnLiteralInt(src, name string) (int64, bool) {
	re := regexp.MustCompile(`fn ` + regexp.QuoteMeta(name) + `\(\) -> \w+ \{\s*(-?(?:0x)?[0-9a-fA-F]+)\s*\}`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseInt(m[1], 0, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// recordFieldInt extracts N from a `<name>: N` line inside a record
// literal, accepting decimal or 0x-hex.
func recordFieldInt(src, name string) (int64, bool) {
	re := regexp.MustCompile(regexp.QuoteMeta(name) + `:\s*(-?(?:0x)?[0-9a-fA-F]+)`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseInt(m[1], 0, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
