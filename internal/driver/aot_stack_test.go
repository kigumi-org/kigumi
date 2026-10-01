package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kigumi/internal/driver"
)

// TestAotLoopStack runs a loop long enough to exhaust the stack if the
// argument buffers of its calls were allocated per iteration.
func TestAotLoopStack(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root := t.TempDir()
	src := `fn fill(n: Int) -> Bytes {
    let mut out = Array.empty[u8]()
    for i in 0..n {
        out.push(7)
    }
    Bytes.fromArray(&out)
}

fn total(data: &Bytes, n: Int) -> Int {
    let mut sum = 0
    for i in 0..n {
        sum = sum + (data.get(i.toUsize() || 0) || 0).toInt()
    }
    sum
}

let data = fill(300000)
print "${total(&data, 300000)} ${total(&data, 300000)}"
`
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"loopstack\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte(src), 0o644)
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
	if string(out) != "2100000 2100000\n" {
		t.Errorf("got %q", out)
	}
}
