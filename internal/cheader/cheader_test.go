package cheader

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func zigOrSkip(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("zig")
	if err != nil {
		t.Skip("no zig")
	}
	return p
}

func writeHeader(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.h")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestImportGeneratesDeclarations covers a struct, an enum, two functions
// and an integer macro: the shapes its own fixture needs.
func TestImportGeneratesDeclarations(t *testing.T) {
	zig := zigOrSkip(t)
	header := writeHeader(t, `
typedef struct Point { int x; int y; } Point;
typedef enum Color { RED = 0, GREEN = 1, BLUE = 2 } Color;
int add(int a, int b);
void greet(const char *name);
#define MAX_ITEMS 64
`)
	res, err := Import(Options{ZigPath: zig, HeaderPath: header, ABI: ABIFor("linux", "amd64"), CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"pub fn add(a: i32, b: i32) -> i32",
		"pub fn greet(name: *const u8) -> Unit",
		"pub type Point layout(C) = {",
		"pub x i32",
		"pub const RED: u32 = 0",
		"pub const MAX_ITEMS: i32 = 64",
	} {
		if !strings.Contains(res.Source, want) {
			t.Errorf("source missing %q\n--- source ---\n%s", want, res.Source)
		}
	}
	if res.FromCache {
		t.Error("first import should not be a cache hit")
	}
}

// TestImportSkipsUnsupported covers the categories requiring a listed,
// non-silent skip.
func TestImportSkipsUnsupported(t *testing.T) {
	zig := zigOrSkip(t)
	header := writeHeader(t, `
#define SQUARE(x) ((x) * (x))
static inline int inline_add(int a, int b) { return a + b; }
extern int global_counter;
`)
	res, err := Import(Options{ZigPath: zig, HeaderPath: header, ABI: ABIFor("linux", "amd64"), CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(res.Source) != "" {
		t.Errorf("expected nothing importable, got:\n%s", res.Source)
	}
	joined := strings.Join(res.Skipped, "\n")
	for _, name := range []string{"SQUARE", "inline_add", "global_counter"} {
		if !strings.Contains(joined, name) {
			t.Errorf("skip list missing %q:\n%s", name, joined)
		}
	}
}

// TestImportCache checks that a second Import for the same header, zig
// version and target reads the cache instead of invoking zig again.
func TestImportCache(t *testing.T) {
	zig := zigOrSkip(t)
	header := writeHeader(t, "int add(int a, int b);\n")
	cacheDir := t.TempDir()
	calls := 0
	run := func(zigPath, target, headerPath string) ([]byte, error) {
		calls++
		return RunZig(zigPath, target, headerPath)
	}
	opts := Options{ZigPath: zig, HeaderPath: header, ABI: ABIFor("linux", "amd64"), CacheDir: cacheDir, Run: run}
	first, err := Import(opts)
	if err != nil {
		t.Fatal(err)
	}
	if first.FromCache {
		t.Fatal("first import should not be cached")
	}
	callsAfterFirst := calls
	second, err := Import(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !second.FromCache {
		t.Error("second import should be a cache hit")
	}
	if calls != callsAfterFirst {
		t.Errorf("zig invoked again on a cache hit: %d calls, want %d", calls, callsAfterFirst)
	}
	if second.Source != first.Source {
		t.Error("cached source differs from the generated one")
	}
}

// TestImportSkipsBitfieldByValue: by-value use of a
// bitfield-demoted opaque type must be skipped, not emitted (sem's E924).
func TestImportSkipsBitfieldByValue(t *testing.T) {
	zig := zigOrSkip(t)
	header := writeHeader(t, `
struct BitfieldThing {
    unsigned int a : 3;
    unsigned int b : 5;
};
void probe_bitfield(struct BitfieldThing t);
void probe_bitfield_ptr(struct BitfieldThing *t);
int add(int a, int b);
`)
	res, err := Import(Options{ZigPath: zig, HeaderPath: header, ABI: ABIFor("linux", "amd64"), CacheDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Source, "probe_bitfield(t: BitfieldThing)") {
		t.Errorf("a by-value opaque parameter must not be emitted:\n%s", res.Source)
	}
	for _, want := range []string{
		"pub type BitfieldThing",
		"pub fn probe_bitfield_ptr(t: *mut BitfieldThing) -> Unit",
		"pub fn add(a: i32, b: i32) -> i32",
	} {
		if !strings.Contains(res.Source, want) {
			t.Errorf("source missing %q\n--- source ---\n%s", want, res.Source)
		}
	}
	joined := strings.Join(res.Skipped, "\n")
	for _, want := range []string{"probe_bitfield", "BitfieldThing", "by value"} {
		if !strings.Contains(joined, want) {
			t.Errorf("skip list missing %q:\n%s", want, joined)
		}
	}
}

func TestABIFor(t *testing.T) {
	if got := ABIFor("windows", "amd64"); !got.LLP64 {
		t.Error("windows/amd64 should be LLP64")
	}
	if got := ABIFor("linux", "amd64"); got.LLP64 {
		t.Error("linux/amd64 should not be LLP64")
	}
	if got := ABIFor("linux", "arm64"); !got.CharUnsigned {
		t.Error("arm64 should default to unsigned char")
	}
	if got := ABIFor("linux", "amd64"); got.CharUnsigned {
		t.Error("amd64 should default to signed char")
	}
}
