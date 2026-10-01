package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// TestAotCorpus builds every testdata/run program ahead of time and compares
// its output with the interpreter's expectations; it is skipped without a
// C compiler.
func TestAotCorpus(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	files, _ := filepath.Glob("../../testdata/run/*.txtar")
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			runAot(t, path)
		})
	}
}

func runAot(t *testing.T, path string) {
	root, wantOut, wantErr, wantExit := loadRunFixture(t, path)
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: "../../std", Entry: "main/main.kg"})
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "prog")
	var buildErr bytes.Buffer
	ok, err := testBuild(m, exe, &buildErr)
	if err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}
	cmd := exec.Command(exe, "one", "two")
	cmd.Dir = t.TempDir()
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	code := 0
	if err := cmd.Run(); err != nil {
		if ee, isExit := err.(*exec.ExitError); isExit {
			code = ee.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	if out.String() != wantOut {
		t.Errorf("stdout mismatch\n--- got ---\n%s--- want ---\n%s", out.String(), wantOut)
	}
	if wantErr != "" && !strings.Contains(errOut.String(), strings.TrimSpace(wantErr)) {
		t.Errorf("stderr mismatch\n--- got ---\n%s--- want ---\n%s", errOut.String(), wantErr)
	}
	if code != wantExit {
		t.Errorf("exit %d, want %d\nstderr:\n%s", code, wantExit, errOut.String())
	}
}

// TestAotFFI exercises the C boundary: libc calls, a C variadic, raw
// pointers through std/ffi, a library from the manifest's Link record, and
// an export(C) function called back from a C helper linked via KIGUMI_CFLAGS.
func TestAotFFI(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root := t.TempDir()
	src := `import ffi from std/ffi

extern(C) {
    fn strlen(s: *const u8) -> usize
    fn sqrt(x: f64) -> f64
    fn snprintf(buf: *mut u8, n: usize, fmt: *const u8, ...) -> i32
    fn malloc(n: usize) -> *mut u8
    fn free(p: *mut u8) -> Unit
    fn call_twice(x: i32) -> i32
    fn apply_i32(f: extern(C) fn(i32) -> i32, x: i32) -> i32
    fn twice_ptr() -> extern(C) fn(i32) -> i32
    fn apply_len(f: extern(C) fn(*const u8) -> usize, s: *const u8) -> usize
}

export(C) {
    pub fn kg_twice(x: i32) -> i32 {
        x * 2
    }
}

pub type Inner layout(C) = {
    pub tag u8
    pub weight f64
}

pub type Point layout(C) = {
    pub x i32
    pub y i64
    pub inner Inner
}

extern(C) {
    fn sum_point(p: *const Point) -> f64
    fn fill_point(p: *mut Point) -> Unit
    fn point_sum(p: Point) -> f64
    fn make_point(x: i32, y: i64) -> Point
    fn call_scale(p: Point) -> Point
    fn apply_tag(f: extern(C) fn(Point) -> u8, p: Point) -> u8
    fn with_userdata(cb: extern(C) fn(*mut u8, i32) -> i32, ud: *mut u8, x: i32) -> i32
}

type Counter = {
    pub base i32
}

export(C) {
    pub fn kg_cb(ud: *mut u8, x: i32) -> i32 {
        unsafe { ffi.Pin.with(ud, (c: &Counter) => c.base + x) }
    }
}

export(C) {
    pub fn kg_scale(p: Point) -> Point {
        Point { x: p.x * 2, y: p.y * 2, inner: p.inner }
    }
    pub fn kg_tag(p: Point) -> u8 {
        p.inner.tag
    }
}

let cv = ffi.CValue.new(Point { x: 3, y: 4, inner: Inner { tag: 1, weight: 2.5 } })
let total = unsafe { sum_point(ffi.toConst(cv.ptr())) }
unsafe {
    fill_point(cv.ptr())
}
let filled = cv.get()
print "${total} ${filled.x} ${filled.y} ${filled.inner.tag} ${filled.inner.weight}"

let hello = "hello"
let s = ffi.CString.new(&hello)?
let n = unsafe { strlen(s.ptr()) }
let buf = unsafe { malloc(32) }
let pattern = "%d-%s-%.1f"
let fmt = ffi.CString.new(&pattern)?
let w = unsafe { snprintf(buf, 32, fmt.ptr(), 7, s.ptr(), 2.5) }
let text = unsafe { ffi.string(ffi.toConst(buf)) }
unsafe {
    ffi.writeU8(buf, 65)
}
let first = unsafe { ffi.readU8(ffi.toConst(buf)) }
unsafe {
    free(buf)
}
let root = unsafe { sqrt(16.0) }
let twice = unsafe { call_twice(21) }
print "${n} ${w} ${text} ${first} ${root} ${twice} ${ffi.isNull(ffi.null[u8]())}"
let viaC = unsafe { apply_i32(kg_twice, 21) }
let tp = unsafe { twice_ptr() }
let viaPtr = unsafe { tp(4) }
let lenVia = unsafe { apply_len(strlen, s.ptr()) }
print "${viaC} ${viaPtr} ${lenVia}"
let mp = unsafe { make_point(2, 3) }
let sum = unsafe { point_sum(mp) }
let scaled = unsafe { call_scale(mp) }
let tag = unsafe { apply_tag(kg_tag, scaled) }
let g: extern(C) fn(Point) -> Point = kg_scale
let again = unsafe { g(scaled) }
print "${mp.x} ${mp.y} ${mp.inner.tag} ${mp.inner.weight} ${sum} ${scaled.x} ${scaled.y} ${tag} ${again.x}"
let pin = ffi.Pin.new(Counter { base: 100 })
print "${unsafe { with_userdata(kg_cb, pin.ptr(), 5) }}"
`
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"ffi\"\n    kigumi: \"0.1\"\n}\nLink { library: \"m\" }\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte(src), 0o644)
	helper := filepath.Join(root, "helper.c")
	cHelper := "#include <stddef.h>\nint kg_twice(int);\nint call_twice(int x) { return kg_twice(x); }\n" +
		"int apply_i32(int (*f)(int), int x) { return f(x); }\nint (*twice_ptr(void))(int) { return kg_twice; }\n" +
		"size_t apply_len(size_t (*f)(const char *), const char *s) { return f(s); }\n" +
		"struct inner { unsigned char tag; double weight; };\nstruct point { int x; long long y; struct inner inner; };\n" +
		"double sum_point(const struct point *p) { return p->x + p->y + p->inner.tag + p->inner.weight; }\n" +
		"void fill_point(struct point *p) { p->x = -7; p->y = 1LL << 40; p->inner.tag = 200; p->inner.weight = 0.25; }\n" +
		"double point_sum(struct point p) { return p.x + p.y + p.inner.tag + p.inner.weight; }\n" +
		"struct point make_point(int x, long long y) { struct point p = { x, y, { 9, 1.5 } }; return p; }\n" +
		"struct point kg_scale(struct point);\nstruct point call_scale(struct point p) { return kg_scale(p); }\n" +
		"unsigned char apply_tag(unsigned char (*f)(struct point), struct point p) { return f(p); }\n" +
		"int with_userdata(int (*cb)(void *, int), void *ud, int x) { return cb(ud, x); }\n"
	os.WriteFile(helper, []byte(cHelper), 0o644)
	t.Setenv("KIGUMI_CFLAGS", helper)
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "prog")
	var buildErr bytes.Buffer
	ok, err := testBuild(m, exe, &buildErr)
	if err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}
	out, err := exec.Command(exe).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	if string(out) != "10.5 -7 1099511627776 200 0.25\n5 11 7-hello-2.5 65 4 42 true\n42 8 5\n2 3 9 1.5 15.5 4 6 9 8\n105\n" {
		t.Errorf("got %q", out)
	}
}
