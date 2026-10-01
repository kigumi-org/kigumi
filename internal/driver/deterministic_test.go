package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"kigumi/internal/driver"
)

// TestDeterministicArchive builds the same freestanding module into an
// archive twice, each time under a fresh os.MkdirTemp build dir and a
// fresh zig cache, and expects byte-identical archives. The random build
// dir must never leak into the objects (via unnamed-type names or
// DW_AT_comp_dir) or into the archive (via member metadata).
func TestDeterministicArchive(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	std, _ := filepath.Abs("../../std")
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"bare\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("print \"hi\"\n"), 0o644)
	target, err := driver.ParseTarget("x86_64-freestanding", "")
	if err != nil {
		t.Fatal(err)
	}

	var archives [][]byte
	for i := range 2 {
		t.Setenv("ZIG_GLOBAL_CACHE_DIR", filepath.Join(t.TempDir(), "zig-global"))
		t.Setenv("ZIG_LOCAL_CACHE_DIR", filepath.Join(t.TempDir(), "zig-local"))
		m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target})
		if err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(t.TempDir(), "bare.a")
		var stderr bytes.Buffer
		if ok, err := driver.BuildWith(m, out, &stderr, driver.BuildOptions{Target: target}); err != nil || !ok {
			t.Fatalf("build %d: %v\n%s", i, err, stderr.String())
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		archives = append(archives, data)
	}
	if !bytes.Equal(archives[0], archives[1]) {
		t.Fatalf("two builds of the same module produced different archives (%d vs %d bytes)", len(archives[0]), len(archives[1]))
	}
}

// TestDeterministicIR compiles the same module twice from scratch, under
// two different roots, and expects identical IR with no entity numbers in
// the symbols the object file would carry.
func TestDeterministicIR(t *testing.T) {
	src := `type Shape = Dot | Box(Int, Int)

type Counter = resource {
    pub n Int
}

fn Counter.drop(move self) -> Unit {
    print "drop ${self.n}"
}

fn describe(s: Shape) -> String {
    match s {
        Shape.Dot -> "dot"
        Shape.Box(w, h) -> "box ${w * h}"
    }
}

fn describe(n: Int) -> String {
    "int ${n}"
}

let c = Counter { n: 1 }
let f = (k: Int) => k + c.n
print "${describe(Shape.Box(2, 3))} ${describe(4)} ${f(1)}"
`
	std, _ := filepath.Abs("../../std")
	var irs []string
	for _, dir := range []string{"first", "second-root"} {
		root := filepath.Join(t.TempDir(), dir)
		os.MkdirAll(root, 0o755)
		os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"det\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
		os.WriteFile(filepath.Join(root, "main.kg"), []byte(src), 0o644)
		m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
		if err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		ir, ok, err := driver.Compile(m, &stderr)
		if err != nil || !ok {
			t.Fatalf("compile: %v\n%s", err, stderr.String())
		}
		irs = append(irs, ir)
	}
	if irs[0] != irs[1] {
		t.Fatal("two compilations of the same module differ")
	}
	for _, bad := range []string{`kigumi\.main#`, `@\.type[0-9]`, `@\.variant[0-9]`, `main#[0-9]`, `closure[0-9]{3,}`} {
		if regexp.MustCompile(bad).MatchString(irs[0]) {
			t.Errorf("IR carries an entity-numbered symbol matching %s", bad)
		}
	}
}
