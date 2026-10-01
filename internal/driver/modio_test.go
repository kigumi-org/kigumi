package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

func cacheEntry(t *testing.T, name, version string, ready bool) string {
	t.Helper()
	dir, err := driver.CacheDir(driver.Require{Name: name, Version: version})
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "mod.kg"), []byte("Module {\n    name: \""+name+"\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "lib.kg"), []byte("pub fn v() -> Int {\n    1\n}\n"), 0o644)
	if ready {
		os.WriteFile(filepath.Join(dir, driver.OriginFileName), []byte("abcdef0\n"), 0o644)
	}
	return dir
}

// A cache entry is used only when it is complete and made of regular
// files: a missing origin marker means an unfinished fetch, and a symlink
// could lead outside the cache.
func TestCacheEntryChecks(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	app := t.TempDir()
	os.WriteFile(filepath.Join(app, "mod.kg"), []byte("Module {\n    name: \"app\"\n    kigumi: \"0.1\"\n}\nRequire { name: \"greet\", version: \"v1.0.0\" }\n"), 0o644)
	os.WriteFile(filepath.Join(app, "main.kg"), []byte("import {v} from greet\nprint \"${v()}\"\n"), 0o644)

	cacheEntry(t, "greet", "v1.0.0", false)
	if _, err := driver.LoadModule(app, driver.LoadOptions{}); err == nil || !strings.Contains(err.Error(), "is not fetched") {
		t.Fatalf("an entry without the origin marker must count as not fetched: %v", err)
	}
	dir := cacheEntry(t, "greet", "v1.0.0", true)
	hash, err := driver.TreeHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.WriteLock(app, driver.Lock{Entries: map[string]driver.LockEntry{
		"greet": {Version: "v1.0.0", Commit: "abcdef0", Hash: hash},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := driver.LoadModule(app, driver.LoadOptions{}); err != nil {
		t.Fatalf("a complete entry must load: %v", err)
	}
	if err := os.Symlink("/etc/hostname", filepath.Join(dir, "extra.kg")); err != nil {
		t.Skip(err)
	}
	if _, err := driver.LoadModule(app, driver.LoadOptions{}); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("a symlink in the cache must be refused: %v", err)
	}
}

// Missing files are the only errors read as "absent": an unreadable
// manifest is reported, and lock writes leave no temporary behind.
func TestModuleIOErrors(t *testing.T) {
	root := t.TempDir()
	if _, ok, err := driver.ReadModFile(root); ok || err != nil {
		t.Fatalf("missing manifest: ok %v err %v", ok, err)
	}
	if os.Getuid() != 0 {
		os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"a\"\n    kigumi: \"0.1\"\n}\n"), 0o000)
		if _, _, err := driver.ReadModFile(root); err == nil {
			t.Fatal("an unreadable manifest must be an error, not a missing one")
		}
		os.Remove(filepath.Join(root, "mod.kg"))
	}
	if err := driver.WriteLock(root, driver.Lock{Entries: map[string]driver.LockEntry{"x": {Version: "v1.0.0", Commit: "c", Hash: "h"}}}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("temporary file left behind: %s", e.Name())
		}
	}
	if _, err := driver.ReadLock(root); err != nil {
		t.Fatal(err)
	}
}

// A chmod +x'd script (a `#!/usr/bin/env kigumi`
// direct-execution entry point) must keep its executable bit when `kigumi
// get` rewrites it to add a Require record.
func TestAppendScriptRequirePreservesExecutableBit(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	script := filepath.Join(t.TempDir(), "tool.kg")
	os.WriteFile(script, []byte("#!/usr/bin/env kigumi\nprint \"hi\"\n"), 0o644)
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := driver.AppendScriptRequire(script, driver.Require{Name: "github.com/acme/greet", Version: "v0.1.0"}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(script)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("executable bit lost: mode is %v", fi.Mode().Perm())
	}
}

// WriteFileAtomic (used by `kigumi fmt -w`) preserves an existing target's
// mode, falling back to the given perm only when the file does not exist yet.
func TestWriteFileAtomicPreservesExistingMode(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	path := filepath.Join(t.TempDir(), "out.txt")
	if err := driver.WriteFileAtomic(path, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := driver.WriteFileAtomic(path, []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("existing mode not preserved: got %v", fi.Mode().Perm())
	}
}
