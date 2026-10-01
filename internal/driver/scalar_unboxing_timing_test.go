package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"kigumi/internal/driver"
)

// TestScalarUnboxingTiming pins a performance win: a hot loop of plain
// Int/Bool/u8/u32 arithmetic keeps a MIR temporary in an LLVM register
// instead of boxing a fresh heap value per intermediate.
func TestScalarUnboxingTiming(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root := t.TempDir()
	main := filepath.Join(root, "main", "main.kg")
	if err := os.MkdirAll(filepath.Dir(main), 0o755); err != nil {
		t.Fatal(err)
	}
	src := `fn main() -> Unit! {
    let n: usize = 262144
    let arr = Array.fill[u8](171, n)
    let mut acc: u8 = 0
    let mut i: usize = 0
    for i < n {
        acc = acc ^ arr[i]
        i = i + 1
    }
    print "acc=${acc}"
}
`
	if err := os.WriteFile(main, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

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

	cmd := exec.Command(exe)
	cmd.Dir = t.TempDir()
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	start := time.Now()
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\n%s", err, errOut.String())
	}
	elapsed := time.Since(start)

	// 262144 copies of 171 XORed together is 0 (an even count of an
	// odd-times-repeated value cancels).
	const want = "acc=0\n"
	if out.String() != want {
		t.Errorf("stdout mismatch: got %q, want %q", out.String(), want)
	}
	// Generous bound for a busy, parallel-lane machine: native registers
	// run this loop in well under 40ms.
	const bound = 3 * time.Second
	if elapsed > bound {
		t.Errorf("256 KiB byte-XOR loop took %s, want under %s", elapsed, bound)
	}
	t.Logf("256 KiB byte-XOR loop in %s", elapsed)
}
