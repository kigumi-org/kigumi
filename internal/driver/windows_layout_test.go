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

// TestWindowsAddrInfoLayout pins std/net/addrinfo_windows.kg's field order
// against mingw's ADDRINFOA (zig ships this header at
// libc/include/any-windows-any/ws2tcpip.h):
//
//	typedef struct addrinfo {
//	  int ai_flags;
//	  int ai_family;
//	  int ai_socktype;
//	  int ai_protocol;
//	  size_t ai_addrlen;
//	  char *ai_canonname;
//	  struct sockaddr *ai_addr;
//	  struct addrinfo *ai_next;
//	} ADDRINFOA,*PADDRINFOA;
func TestWindowsAddrInfoLayout(t *testing.T) {
	t.Parallel()
	fields := parseLayoutCFields(t, windowsFile(t, "net", "addrinfo_windows.kg"), "AddrInfo")
	wantOrder := []string{"flags", "family", "socktype", "protocol", "addrlen", "canonname", "addr", "next"}
	var gotOrder []string
	for _, f := range fields {
		gotOrder = append(gotOrder, f.name)
	}
	if strings.Join(gotOrder, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("field order = %v, want %v (ADDRINFOA declares ai_canonname before ai_addr)", gotOrder, wantOrder)
	}

	offsets := layoutCOffsets(fields, windowsAmd64Size)
	wantOffsets := map[string]int64{
		"flags": 0, "family": 4, "socktype": 8, "protocol": 12,
		"addrlen": 16, "canonname": 24, "addr": 32, "next": 40,
	}
	for name, want := range wantOffsets {
		if got := offsets[name]; got != want {
			t.Errorf("offset of %s = %d, want %d (sizeof(ADDRINFOA) on x64 is 48)", name, got, want)
		}
	}
}

// TestWindowsDirentNameOffset pins std/fs/dirent_windows.kg against mingw's
// dirent.h (any-windows-any/dirent.h):
//
//	struct dirent {
//	    long           d_ino;     /* Always zero. (long stays 32 bit on Win64) */
//	    unsigned short d_reclen;  /* Always zero. */
//	    unsigned short d_namlen;  /* Length of name in d_name. */
//	    char           d_name[260];
//	};
func TestWindowsDirentNameOffset(t *testing.T) {
	t.Parallel()
	src := windowsFile(t, "fs", "dirent_windows.kg")
	m := regexp.MustCompile(`direntNameOffset\(\) -> Int \{\s*(\d+)\s*\}`).FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("could not find direntNameOffset's literal body in:\n%s", src)
	}
	got, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if want := 4 + 2 + 2; got != want {
		t.Errorf("direntNameOffset() = %d, want %d (d_ino 4 + d_reclen 2 + d_namlen 2)", got, want)
	}
}

// TestWindowsPlatformFilesCheck compiles std/net and std/fs for
// x86_64-windows with the checker (no linker needed), so a layout(C)
// mistake in a windows platform file is a normal compile error rather than
// something only a real Windows run would surface.
func TestWindowsPlatformFilesCheck(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"winchk\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("import net from std/net\nimport fs from std/fs\nprint \"ok\"\n"), 0o644)
	std, err := filepath.Abs("../../std")
	if err != nil {
		t.Fatal(err)
	}
	target, err := driver.ParseTarget("x86_64-windows", "")
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
		t.Fatalf("x86_64-windows checking failed:\n%s", res.Render())
	}
}

func windowsFile(t *testing.T, pkg, name string) string {
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

type layoutCField struct {
	name, typ string
}

// parseLayoutCFields extracts the `pub <name> <type>` fields of a
// `type <name> layout(C) = { ... }` record, in source order.
func parseLayoutCFields(t *testing.T, src, typeName string) []layoutCField {
	t.Helper()
	start := strings.Index(src, "type "+typeName+" layout(C) = {")
	if start < 0 {
		t.Fatalf("no `type %s layout(C)` in source", typeName)
	}
	end := strings.Index(src[start:], "}")
	if end < 0 {
		t.Fatalf("unterminated %s record", typeName)
	}
	var fields []layoutCField
	for _, line := range strings.Split(src[start:start+end], "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "pub ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "pub "))
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) != 2 {
			t.Fatalf("unexpected field line %q", line)
		}
		fields = append(fields, layoutCField{name: parts[0], typ: strings.TrimSpace(parts[1])})
	}
	return fields
}

// windowsAmd64Size is the C ABI size of a layout(C) field on x86_64-windows
// (LLP64): fixed-width integers are their bit width, anything else (a raw
// pointer) is 8 bytes.
func windowsAmd64Size(typ string) int64 {
	switch typ {
	case "i8", "u8":
		return 1
	case "i16", "u16":
		return 2
	case "i32", "u32":
		return 4
	case "i64", "u64", "usize", "isize":
		return 8
	default:
		return 8
	}
}

// layoutCOffsets is the byte offset of each field under natural alignment,
// the rule internal/llgen's cOffsets and internal/interp's cLayout use for
// a layout(C) record (every field aligns to its own size here, true for
// every field type these two structs use).
func layoutCOffsets(fields []layoutCField, sizeOf func(string) int64) map[string]int64 {
	offsets := make(map[string]int64, len(fields))
	var off int64
	for _, f := range fields {
		size := sizeOf(f.typ)
		off = (off + size - 1) / size * size
		offsets[f.name] = off
		off += size
	}
	return offsets
}
