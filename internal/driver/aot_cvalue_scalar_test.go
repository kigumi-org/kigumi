package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kigumi/internal/driver"
)

// TestAotCValueScalar:
// `ffi.CValue.new[T]` with a scalar T must allocate real C memory and
// move the value through it, not an empty record, so a genuine C function
// writing through the pointer as an out parameter (and reading one
// `.set()` by Kigumi wrote) sees the same bytes `.get()` does.
func TestAotCValueScalar(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root := t.TempDir()
	src := `import ffi from std/ffi

extern(C) {
    fn get_i64(p: *const i64) -> i64
    fn set_i64(p: *mut i64, v: i64) -> Unit
    fn get_u32(p: *const u32) -> u32
    fn set_u32(p: *mut u32, v: u32) -> Unit
    fn get_u8(p: *const u8) -> u8
    fn set_u8(p: *mut u8, v: u8) -> Unit
    fn get_f64(p: *const f64) -> f64
    fn set_f64(p: *mut f64, v: f64) -> Unit
    fn get_bool_byte(p: *const Bool) -> u8
    fn set_bool_byte(p: *mut Bool, v: u8) -> Unit
}

let iv = ffi.CValue.new[i64](7)
let seenI = unsafe { get_i64(ffi.toConst(iv.ptr())) }
unsafe { set_i64(iv.ptr(), 1000) }
let afterSetI = iv.get()
iv.set(&(-5))
let seenI2 = unsafe { get_i64(ffi.toConst(iv.ptr())) }

let uv = ffi.CValue.new[u32](3)
let seenU = unsafe { get_u32(ffi.toConst(uv.ptr())) }
unsafe { set_u32(uv.ptr(), 4000000000) }
let afterSetU = uv.get()
uv.set(&123456)
let seenU2 = unsafe { get_u32(ffi.toConst(uv.ptr())) }

let bv = ffi.CValue.new[u8](5)
let seenB = unsafe { get_u8(ffi.toConst(bv.ptr())) }
unsafe { set_u8(bv.ptr(), 250) }
let afterSetB = bv.get()
bv.set(&9)
let seenB2 = unsafe { get_u8(ffi.toConst(bv.ptr())) }

let fv = ffi.CValue.new[f64](1.5)
let seenF = unsafe { get_f64(ffi.toConst(fv.ptr())) }
unsafe { set_f64(fv.ptr(), 2.75) }
let afterSetF = fv.get()
fv.set(&(-3.5))
let seenF2 = unsafe { get_f64(ffi.toConst(fv.ptr())) }

let boolv = ffi.CValue.new[Bool](true)
let seenBool = unsafe { get_bool_byte(ffi.toConst(boolv.ptr())) }
unsafe { set_bool_byte(boolv.ptr(), 0) }
let afterSetBool = boolv.get()
boolv.set(&true)
let seenBool2 = unsafe { get_bool_byte(ffi.toConst(boolv.ptr())) }

print "${seenI} ${afterSetI} ${seenI2}"
print "${seenU} ${afterSetU} ${seenU2}"
print "${seenB} ${afterSetB} ${seenB2}"
print "${seenF} ${afterSetF} ${seenF2}"
print "${seenBool} ${afterSetBool} ${seenBool2}"
`
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"cvaluescalar\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte(src), 0o644)
	helper := filepath.Join(root, "helper.c")
	cHelper := "" +
		"long long get_i64(const long long *p) { return *p; }\n" +
		"void set_i64(long long *p, long long v) { *p = v; }\n" +
		"unsigned int get_u32(const unsigned int *p) { return *p; }\n" +
		"void set_u32(unsigned int *p, unsigned int v) { *p = v; }\n" +
		"unsigned char get_u8(const unsigned char *p) { return *p; }\n" +
		"void set_u8(unsigned char *p, unsigned char v) { *p = v; }\n" +
		"double get_f64(const double *p) { return *p; }\n" +
		"void set_f64(double *p, double v) { *p = v; }\n" +
		"unsigned char get_bool_byte(const unsigned char *p) { return *p; }\n" +
		"void set_bool_byte(unsigned char *p, unsigned char v) { *p = v; }\n"
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
	want := "7 1000 -5\n3 4000000000 123456\n5 250 9\n1.5 2.75 -3.5\n1 false 1\n"
	if string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
