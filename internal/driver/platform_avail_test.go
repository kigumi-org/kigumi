package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

func writePlatformModule(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \""+name+"\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("import time from std/time\n\nfn age(t: time.Instant) -> Unit {}\n"), 0o644)
	return root
}

// TestPlatformAvailableHosted checks that a hosted build still imports
// std/time without error (request 8's baseline).
func TestPlatformAvailableHosted(t *testing.T) {
	t.Parallel()
	root := writePlatformModule(t, "hosted")
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.HasErrors() {
		t.Fatalf("hosted build should import std/time fine:\n%s", res.Render())
	}
}

// TestPlatformUnavailableFreestanding rejects std/time on a freestanding
// target, generalizing the std/dl case (TestDLFreestanding) to the
// package-to-layer table.
func TestPlatformUnavailableFreestanding(t *testing.T) {
	t.Parallel()
	root := writePlatformModule(t, "bare")
	target, _ := driver.ParseTarget("x86_64-freestanding", "")
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasErrors() || !strings.Contains(res.Render(), "`std/time` is not available on a freestanding target") {
		t.Fatalf("expected the freestanding diagnostic:\n%s", res.Render())
	}
}

// TestPlatformUnavailableSysNone rejects std/time under `--sys none` even
// on a hosted target: the profile that is not just Target.Freestanding().
func TestPlatformUnavailableSysNone(t *testing.T) {
	t.Parallel()
	root := writePlatformModule(t, "sysnone")
	target, err := driver.ParseTarget("", "none")
	if err != nil {
		t.Fatal(err)
	}
	if target.Freestanding() {
		t.Fatal("a hosted triple with --sys none must not become freestanding")
	}
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasErrors() || !strings.Contains(res.Render(), "`std/time` is not available with `--sys none`") {
		t.Fatalf("expected the --sys none diagnostic:\n%s", res.Render())
	}
}

// TestPlatformUnavailableRandomFreestanding rejects std/random on a
// freestanding target: fromClock() reads std/time, so random is Platform
// like time itself, and a user who
// never calls fromClock still cannot build for freestanding today.
func TestPlatformUnavailableRandomFreestanding(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"randbare\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("import random from std/random\n\nfn seed(r: random.Rng) -> Unit {}\n"), 0o644)
	target, _ := driver.ParseTarget("x86_64-freestanding", "")
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasErrors() || !strings.Contains(res.Render(), "`std/random` is not available on a freestanding target") {
		t.Fatalf("expected the freestanding diagnostic:\n%s", res.Render())
	}
}

// TestPlatformUnavailableTransitive checks availability is transitive: a
// Foundation package with no Platform layer
// of its own that reaches std/time only through another std package must
// still be rejected, naming std/time at the user's own import rather than
// the directly-imported package. The overlay adds a synthetic probe file to
// std/crypto so the violation does not depend on any real package's
// current imports.
func TestPlatformUnavailableTransitive(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"transitive\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("import crypto from std/crypto\n\nfn use() -> Unit { crypto.touchClock() }\n"), 0o644)
	target, _ := driver.ParseTarget("x86_64-freestanding", "")
	std, _ := filepath.Abs("../../std")
	overlay := map[string][]byte{
		filepath.Join(std, "crypto", "layer_probe.kg"): []byte("import time from std/time\n\npub fn touchClock() -> Unit {\n    _ = time.now()\n}\n"),
	}
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target, Overlay: overlay})
	if err != nil {
		t.Fatal(err)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	rendered := res.Render()
	if !res.HasErrors() || !strings.Contains(rendered, "`std/time` is not available on a freestanding target") {
		t.Fatalf("expected the transitive std/time diagnostic:\n%s", rendered)
	}
	if !strings.Contains(rendered, "main.kg:1") {
		t.Fatalf("expected the diagnostic at main.kg's own import, not the synthetic std/crypto file:\n%s", rendered)
	}
}

// TestStdLayerDirectionFreestanding walks the whole std tree (every
// package gets checked regardless of what the entry imports, sem.go
// addPackages) under a freestanding target, so it sees every `_bare.kg`
// file's imports too: none may reach a higher std layer than its own
// package. TestStdStubs (internal/sem)
// covers the same rule for the host-platform / `_hosted.kg` files.
func TestStdLayerDirectionFreestanding(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"stdwalk\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("fn main() -> Unit! {}\n"), 0o644)
	target, _ := driver.ParseTarget("x86_64-freestanding", "")
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	if out := res.Render(); out != "" {
		t.Errorf("std's bare/freestanding files are not clean:\n%s", out)
	}
}
