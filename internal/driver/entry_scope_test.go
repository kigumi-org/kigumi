package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

func writeEntryScopeModule(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestBuildIgnoresSiblingCmdEntry covers request 5: building one cmd/*
// entry for a freestanding target must not load a sibling cmd/* entry, so
// the sibling's use of `host` (unavailable freestanding) never surfaces as
// "exists only in the entry file".
func TestBuildIgnoresSiblingCmdEntry(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeEntryScopeModule(t, root, map[string]string{
		"mod.kg":              "Module {\n    name: \"twoentries\"\n    kigumi: \"0.1\"\n}\n",
		"cmd/app/main.kg":     "let out = host.args().get(1) || \"missing\"\nprint out\n",
		"cmd/enclave/main.kg": "print \"hello from the enclave\"\n",
	})
	target, err := driver.ParseTarget("x86_64-freestanding", "")
	if err != nil {
		t.Fatal(err)
	}
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Entry: "cmd/enclave/main.kg", Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if d := m.Diagnostics(); d != "" {
		t.Fatalf("unexpected parse diagnostics: %s", d)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.HasErrors() {
		t.Fatalf("cmd/enclave should build clean of the sibling's host use:\n%s", res.Render())
	}
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	out := filepath.Join(root, "enclave.a")
	var stderr bytes.Buffer
	ok, err := driver.BuildWith(m, out, &stderr, driver.BuildOptions{Target: target})
	if err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil || !bytes.HasPrefix(data, []byte("!<arch>")) {
		t.Fatalf("expected a static archive: %v", err)
	}
}

// TestBuildIgnoresSiblingCmdEntryParseError covers the gate `kigumi build`
// hits before checking: a sibling cmd/* entry's parse error must not
// surface either, since it is not part of this build.
func TestBuildIgnoresSiblingCmdEntryParseError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeEntryScopeModule(t, root, map[string]string{
		"mod.kg":              "Module {\n    name: \"parseerr\"\n    kigumi: \"0.1\"\n}\n",
		"cmd/app/main.kg":     "let args = host.args(\n",
		"cmd/enclave/main.kg": "print \"hello from the enclave\"\n",
	})
	target, err := driver.ParseTarget("x86_64-freestanding", "")
	if err != nil {
		t.Fatal(err)
	}
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Entry: "cmd/enclave/main.kg", Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if d := m.Diagnostics(); d != "" {
		t.Fatalf("cmd/enclave should ignore the sibling's syntax error: %s", d)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.HasErrors() {
		t.Fatalf("cmd/enclave should check clean:\n%s", res.Render())
	}
	whole, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	if d := whole.Diagnostics(); !strings.Contains(d, "cmd/app/main.kg") {
		t.Fatalf("whole-module load should still report the sibling's syntax error: %s", d)
	}
}

// TestCheckWholeModuleChecksEveryCmdEntry keeps `kigumi check` on a whole
// module checking every cmd/* package as its own root, each reporting its
// own errors independently.
func TestCheckWholeModuleChecksEveryCmdEntry(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeEntryScopeModule(t, root, map[string]string{
		"mod.kg":              "Module {\n    name: \"twoerrs\"\n    kigumi: \"0.1\"\n}\n",
		"cmd/app/main.kg":     "let x = onlyInApp\n",
		"cmd/enclave/main.kg": "let y = onlyInEnclave\n",
	})
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Render()
	if !strings.Contains(out, "onlyInApp") || !strings.Contains(out, "onlyInEnclave") {
		t.Fatalf("whole-module check should report both cmd/* entries' errors:\n%s", out)
	}
}
