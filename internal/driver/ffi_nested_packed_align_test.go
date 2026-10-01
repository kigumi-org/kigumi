package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"kigumi/internal/driver"
)

// TestFFINestedPackedAlign pins layout-attributes-1: a plain layout(C)
// record nested inside a layout(packed) record must have its own
// generated reader/writer honor the reduced alignment the outer packing
// gives its address, not the record's own natural alignment. Wrong
// values don't surface in testdata/run/ffi_nested_packed.txtar on
// x86-64 at -O1 (the host tolerates the misaligned access), so this
// inspects the generated LLVM IR align annotations directly.
func TestFFINestedPackedAlign(t *testing.T) {
	src := `import ffi from std/ffi

type Inner layout(C) = {
    pub x i64
    pub y i64
}

type Outer layout(packed) = {
    pub tag u8
    pub inner Inner
}

let o = Outer { tag: 9, inner: Inner { x: 111, y: 222 } }
let cv = ffi.CValue.new(o)
print "${cv.get().inner.x}"
`
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"align\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	ir, ok, err := driver.Compile(m, &stderr)
	if err != nil || !ok {
		t.Fatalf("compile: %v\n%s", err, stderr.String())
	}
	fn := regexp.MustCompile(`(?s)define (?:ptr|void) @kg_c(?:read|write)_\S*Inner\S*\(.*?\n\}\n`)
	matches := fn.FindAllString(ir, -1)
	if len(matches) == 0 {
		t.Fatal("no generated reader/writer found for the nested Inner record")
	}
	align8 := regexp.MustCompile(`align 8`)
	for _, body := range matches {
		if align8.MatchString(body) {
			t.Errorf("nested Inner reader/writer still claims align 8 despite the packed outer:\n%s", body)
		}
	}
}
