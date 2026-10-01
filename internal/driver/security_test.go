package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// A dependency's own mod.local.kg is never read: only the override next to
// the root module takes effect, however the dependency itself was reached
// (a real fetch, or the root's own override).
func TestReplaceIgnoredBeyondRoot(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	mid := filepath.Join(root, "mid")
	secret := filepath.Join(root, "secret")
	os.MkdirAll(app, 0o755)
	os.MkdirAll(mid, 0o755)
	os.MkdirAll(secret, 0o755)
	os.WriteFile(filepath.Join(secret, "secret.kg"), []byte("pub const key: String = \"leaked\"\n"), 0o644)
	os.WriteFile(filepath.Join(mid, "mod.kg"), []byte("Module {\n    name: \"mid\"\n    kigumi: \"0.1\"\n}\nRequire { name: \"secret\", version: \"v1.0.0\" }\n"), 0o644)
	os.WriteFile(filepath.Join(mid, "mod.local.kg"), []byte("Replace { name: \"secret\", path: \""+filepath.ToSlash(secret)+"\" }\n"), 0o644)
	os.WriteFile(filepath.Join(mid, "mid.kg"), []byte("pub fn noop() -> Int {\n    0\n}\n"), 0o644)
	os.WriteFile(filepath.Join(app, "mod.kg"), []byte("Module {\n    name: \"app\"\n    kigumi: \"0.1\"\n}\nRequire { name: \"mid\", version: \"v1.0.0\" }\n"), 0o644)
	os.WriteFile(filepath.Join(app, "mod.local.kg"), []byte("Replace { name: \"mid\", path: \""+filepath.ToSlash(mid)+"\" }\n"), 0o644)
	os.WriteFile(filepath.Join(app, "main.kg"), []byte("import {key} from secret\nprint key\n"), 0o644)

	_, err := driver.LoadModule(app, driver.LoadOptions{})
	if err == nil {
		t.Fatal("a non-root Replace should be rejected, not honored")
	}
	if !strings.Contains(err.Error(), "secret v1.0.0 is not fetched") {
		t.Fatalf("wrong diagnostic: %v", err)
	}
}

// kigumi run/build/check must catch a cached dependency that no longer
// matches mod.lock.kg, the same mismatch `kigumi mod verify` reports.
func TestLoadRejectsTamperedCache(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	os.MkdirAll(app, 0o755)
	os.WriteFile(filepath.Join(app, "mod.kg"), []byte("Module {\n    name: \"app\"\n    kigumi: \"0.1\"\n}\nRequire { name: \"greet\", version: \"v1.0.0\" }\n"), 0o644)
	os.WriteFile(filepath.Join(app, "main.kg"), []byte("import {greeting} from greet\nprint greeting()\n"), 0o644)

	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	dir, err := driver.CacheDir(driver.Require{Name: "greet", Version: "v1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "mod.kg"), []byte("Module {\n    name: \"greet\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "greet.kg"), []byte("pub fn greeting() -> String {\n    \"hello\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, driver.OriginFileName), []byte("deadbeef\n"), 0o644)

	hash, err := driver.TreeHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.WriteLock(app, driver.Lock{Entries: map[string]driver.LockEntry{
		"greet": {Version: "v1.0.0", Commit: "deadbeef", Hash: hash},
	}}); err != nil {
		t.Fatal(err)
	}

	if _, err := driver.LoadModule(app, driver.LoadOptions{}); err != nil {
		t.Fatalf("cache matches the lock, should load: %v", err)
	}

	os.WriteFile(filepath.Join(dir, "greet.kg"), []byte("pub fn greeting() -> String {\n    \"TAMPERED\"\n}\n"), 0o644)

	_, err = driver.LoadModule(app, driver.LoadOptions{})
	if err == nil {
		t.Fatal("a cache diverged from mod.lock.kg should be rejected")
	}
	if !strings.Contains(err.Error(), driver.LockName) {
		t.Fatalf("wrong diagnostic: %v", err)
	}
}

// kigumi run/build/check covers MOD-007: a dependency with no entry in
// mod.lock.kg at all must not skip hash verification silently, the same
// as a tampered cache that IS locked.
func TestLoadRejectsMissingLockEntry(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	os.MkdirAll(app, 0o755)
	os.WriteFile(filepath.Join(app, "mod.kg"), []byte("Module {\n    name: \"app\"\n    kigumi: \"0.1\"\n}\nRequire { name: \"greet\", version: \"v1.0.0\" }\n"), 0o644)
	os.WriteFile(filepath.Join(app, "main.kg"), []byte("import {greeting} from greet\nprint greeting()\n"), 0o644)

	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	dir, err := driver.CacheDir(driver.Require{Name: "greet", Version: "v1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "mod.kg"), []byte("Module {\n    name: \"greet\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "greet.kg"), []byte("pub fn greeting() -> String {\n    \"hello\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, driver.OriginFileName), []byte("deadbeef\n"), 0o644)

	_, err = driver.LoadModule(app, driver.LoadOptions{})
	if err == nil {
		t.Fatal("a dependency absent from mod.lock.kg should be rejected")
	}
	if !strings.Contains(err.Error(), "not in") || !strings.Contains(err.Error(), "kigumi get") {
		t.Fatalf("wrong diagnostic: %v", err)
	}
}

// kigumi run/build/check covers MOD-007: a lock entry for a different
// version than what is actually required must not be treated as covering
// the requirement; hash verification must still run, and fail closed.
func TestLoadRejectsStaleLockVersion(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	os.MkdirAll(app, 0o755)
	os.WriteFile(filepath.Join(app, "mod.kg"), []byte("Module {\n    name: \"app\"\n    kigumi: \"0.1\"\n}\nRequire { name: \"greet\", version: \"v1.0.0\" }\n"), 0o644)
	os.WriteFile(filepath.Join(app, "main.kg"), []byte("import {greeting} from greet\nprint greeting()\n"), 0o644)

	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	dir, err := driver.CacheDir(driver.Require{Name: "greet", Version: "v1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "mod.kg"), []byte("Module {\n    name: \"greet\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "greet.kg"), []byte("pub fn greeting() -> String {\n    \"TAMPERED\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, driver.OriginFileName), []byte("deadbeef\n"), 0o644)

	if err := driver.WriteLock(app, driver.Lock{Entries: map[string]driver.LockEntry{
		"greet": {Version: "v0.9.0", Commit: "deadbeef", Hash: "h1:0000000000000000000000000000000000000000000000000000000000000000"},
	}}); err != nil {
		t.Fatal(err)
	}

	_, err = driver.LoadModule(app, driver.LoadOptions{})
	if err == nil {
		t.Fatal("a lock entry for a different version should not cover the requirement")
	}
	if !strings.Contains(err.Error(), "not in") || !strings.Contains(err.Error(), "kigumi get") {
		t.Fatalf("wrong diagnostic: %v", err)
	}
}

// Script mode's lock is opt-in: a script with no
// <script>.lock.kg must still run from a cached dependency, unlike a
// module whose mod.lock.kg `kigumi get` always writes.
func TestScriptRunsWithoutLockFile(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "tool.kg")
	os.WriteFile(script, []byte("Kigumi { version: \"0.1\" }\nRequire { name: \"greet\", version: \"v1.0.0\" }\nimport {greeting} from greet\nprint greeting()\n"), 0o644)

	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	dir, err := driver.CacheDir(driver.Require{Name: "greet", Version: "v1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "mod.kg"), []byte("Module {\n    name: \"greet\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "greet.kg"), []byte("pub fn greeting() -> String {\n    \"hello\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, driver.OriginFileName), []byte("deadbeef\n"), 0o644)

	if _, err := driver.LoadModule(root, driver.LoadOptions{}); err != nil {
		t.Fatalf("a script with no lock file should still load from the cache: %v", err)
	}

	if err := driver.WriteLockFile(script+".lock.kg", driver.Lock{Entries: map[string]driver.LockEntry{
		"greet": {Version: "v0.9.0", Commit: "deadbeef", Hash: "h1:0000000000000000000000000000000000000000000000000000000000000000"},
	}}); err != nil {
		t.Fatal(err)
	}
	_, err = driver.LoadModule(root, driver.LoadOptions{})
	if err == nil {
		t.Fatal("once a script.lock.kg exists, a stale entry should still be rejected")
	}
	if !strings.Contains(err.Error(), "not in") {
		t.Fatalf("wrong diagnostic: %v", err)
	}
}

// Build must not blanket-silence the C compiler: a broken FFI helper linked
// through KIGUMI_CFLAGS should surface a real diagnostic, not a mystery
// link failure.
func TestBuildSurfacesCCompilerDiagnostics(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"t\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("extern(C) {\n    fn call_it() -> i32\n}\nprint \"${unsafe { call_it() }}\"\n"), 0o644)
	helper := filepath.Join(root, "helper.c")
	os.WriteFile(helper, []byte("int call_it(void) { return undeclared_helper(); }\n"), 0o644)
	t.Setenv("KIGUMI_CFLAGS", helper)

	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	var errOut bytes.Buffer
	ok, err := testBuild(m, filepath.Join(root, "prog"), &errOut)
	if err == nil || ok {
		t.Fatal("a broken C helper should fail the build")
	}
	if !strings.Contains(errOut.String(), "undeclared_helper") {
		t.Fatalf("the compiler's own diagnostic should reach the user, got:\n%s", errOut.String())
	}
}

// A dependency's Source or Link path is confined to its own tree even when
// a symlink inside the tree points elsewhere: the manifest's lexical check
// is repeated on the resolved path, for a file and for a directory entry.
func TestNativeSymlinkEscapeRejected(t *testing.T) {
	for _, tc := range []struct{ name, source, link string }{
		{"file", "vendored.c", "vendored.c"},
		{"dir", "csrc", "csrc/evil.c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			app := filepath.Join(root, "app")
			dep := filepath.Join(root, "dep")
			outside := filepath.Join(root, "outside")
			os.MkdirAll(app, 0o755)
			os.MkdirAll(filepath.Join(dep, "csrc"), 0o755)
			os.MkdirAll(outside, 0o755)
			os.WriteFile(filepath.Join(outside, "evil.c"), []byte("int kg_evil(void) { return 1; }\n"), 0o644)
			if err := os.Symlink(filepath.Join(outside, "evil.c"), filepath.Join(dep, tc.link)); err != nil {
				t.Skip(err)
			}
			os.WriteFile(filepath.Join(dep, "mod.kg"), []byte("Module {\n    name: \"dep\"\n    kigumi: \"0.1\"\n}\nSource { c: \""+tc.source+"\" }\n"), 0o644)
			os.WriteFile(filepath.Join(dep, "dep.kg"), []byte("pub fn greet() -> Int {\n    1\n}\n"), 0o644)
			os.WriteFile(filepath.Join(app, "mod.kg"), []byte("Module {\n    name: \"app\"\n    kigumi: \"0.1\"\n}\nRequire { name: \"dep\", version: \"v1.0.0\" }\n"), 0o644)
			os.WriteFile(filepath.Join(app, "mod.local.kg"), []byte("Replace { name: \"dep\", path: \""+filepath.ToSlash(dep)+"\" }\n"), 0o644)
			os.WriteFile(filepath.Join(app, "main.kg"), []byte("import dep from dep\nprint \"${dep.greet()}\"\n"), 0o644)

			_, err := driver.LoadModule(app, driver.LoadOptions{})
			if err == nil || !strings.Contains(err.Error(), "resolves outside the module") {
				t.Fatalf("a Source escaping through a symlink should be rejected, got %v", err)
			}
		})
	}
}
