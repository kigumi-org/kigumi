package llgen_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"kigumi/internal/sem"
	"kigumi/internal/testkit"
)

var (
	exactKey   = regexp.MustCompile(`strcmp\(k, "([^"]+)"\)`)
	patternKey = regexp.MustCompile(`strstr\(k, "([^"]+)"\)`)
	// methodKey matches rt_std_op_of's trailing-method checks (`strcmp(m,
	// "equals")` and friends); m is the key's last dot-segment.
	methodKey = regexp.MustCompile(`strcmp\(m, "([^"]+)"\)`)
)

// lowered lists the primitives llgen expands at the call site, so the
// runtime has no entry for them.
var lowered = map[string]bool{
	"ffi.sizeOf": true, "ffi.alignOf": true, "ffi.readRecord": true, "ffi.writeRecord": true,
	"ffi.CValue.new": true, "ffi.CValue.ptr": true, "ffi.CValue.get": true, "ffi.CValue.set": true, "ffi.CValue.drop": true,
	"ffi.readU8": true, "ffi.readI32": true, "ffi.readI64": true, "ffi.readF64": true, "ffi.readPtr": true,
	"ffi.writeU8": true, "ffi.writeI32": true, "ffi.writeI64": true, "ffi.writeF64": true, "ffi.writePtr": true,
	"ffi.null": true, "ffi.isNull": true, "ffi.cast": true, "ffi.toConst": true, "ffi.toMut": true, "ffi.offset": true,
	"error.as": true,
}

// conversionKey matches the numeric `toX` methods, which callEntity lowers
// to rt_convert.
func conversionKey(key string) bool {
	return strings.HasPrefix(key, "prelude.") && strings.Contains(key, ".to") && !strings.HasPrefix(key, "prelude.String")
}

// TestStdPrimitives pins the contract between std and the C runtime:
// rt_std answers every bodiless std function and no others.
func TestStdPrimitives(t *testing.T) {
	src, err := runtimeText()
	if err != nil {
		t.Fatal(err)
	}
	exact := map[string]bool{}
	for _, m := range exactKey.FindAllStringSubmatch(string(src), -1) {
		exact[m[1]] = true
	}
	var patterns []string
	for _, m := range patternKey.FindAllStringSubmatch(string(src), -1) {
		patterns = append(patterns, m[1])
	}
	for _, m := range methodKey.FindAllStringSubmatch(string(src), -1) {
		patterns = append(patterns, "."+m[1])
	}
	res := sem.Check(&sem.Module{Packages: testkit.LoadStd(t, "../../std")})
	if res.HasErrors() {
		t.Fatalf("check errors:\n%s", res.Render())
	}
	want := map[string]bool{}
	for id := range res.Entities {
		key, ok := primitiveKey(res, sem.EntityID(id))
		if !ok {
			continue
		}
		key = strings.TrimPrefix(key, "std/")
		want[key] = true
		covered := exact[key]
		for _, p := range patterns {
			covered = covered || strings.Contains(key, p)
		}
		if !covered && !lowered[key] && !conversionKey(key) {
			t.Errorf("%s: bodiless std function without a C runtime entry", key)
		}
	}
	if len(want) < 40 {
		t.Fatalf("only %d bodiless std functions found; std not loaded as std?", len(want))
	}
	for key := range exact {
		if !want[key] {
			t.Errorf("%s: C runtime entry for a function that is not a bodiless std function", key)
		}
	}
}

func primitiveKey(res *sem.Result, fn sem.EntityID) (string, bool) {
	e := res.Entity(fn)
	if e.Kind != sem.EntFn || e.Flags&sem.EfStd == 0 || e.File == 0 || e.Name == "branch" {
		return "", false
	}
	info := res.Fn(fn)
	if info.Body != 0 || info.Abi != "" || res.Entity(e.Parent).Kind == sem.EntInterface {
		return "", false
	}
	key := res.Packages[e.Pkg].Path + "."
	if info.Owner != 0 {
		key += res.Entity(info.Owner).Name + "."
	}
	return key + e.Name, true
}

// runtimeText concatenates the runtime's C files, whichever layer a
// primitive lives in.
func runtimeText() ([]byte, error) {
	var out []byte
	for _, name := range []string{"rt_core.c", "rt_sys.c"} {
		b, err := os.ReadFile(filepath.Join("runtime", name))
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	return out, nil
}
