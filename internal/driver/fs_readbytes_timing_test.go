package driver_test

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"kigumi/internal/driver"
)

// TestReadBytesNativeTiming pins the fix for Files.readBytes' O(n^2) byte
// loop: a 6 MB read must finish quickly and
// must not corrupt the data.
func TestReadBytesNativeTiming(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	dataPath := filepath.Join(t.TempDir(), "data.bin")
	data := make([]byte, 6*1024*1024)
	rand.New(rand.NewSource(1)).Read(data)
	if err := os.WriteFile(dataPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	main := filepath.Join(root, "main", "main.kg")
	if err := os.MkdirAll(filepath.Dir(main), 0o755); err != nil {
		t.Fatal(err)
	}
	src := fmt.Sprintf(`import fs from std/fs

let files = host.files()
let p = fs.Path.fromString(%q)?
let data = files.readBytes(p)?
let mid = data.len() / 2
print "len=${data.len()} first=${data.get(0) || 0} mid=${data.get(mid) || 0} last=${data.get(data.len() - 1) || 0}"
`, dataPath)
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

	want := fmt.Sprintf("len=%d first=%d mid=%d last=%d\n", len(data), data[0], data[len(data)/2], data[len(data)-1])
	if out.String() != want {
		t.Errorf("stdout mismatch\n--- got ---\n%s--- want ---\n%s", out.String(), want)
	}
	// Generous bound for a busy, parallel-lane machine; the byte-by-byte
	// concat this replaces took over 60s for a 6 MB file on a slow CPU.
	const bound = 20 * time.Second
	if elapsed > bound {
		t.Errorf("readBytes took %s for 6 MB, want under %s", elapsed, bound)
	}
	t.Logf("read 6 MB natively in %s", elapsed)
}
