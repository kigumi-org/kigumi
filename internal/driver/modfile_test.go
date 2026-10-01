package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
	"kigumi/internal/token"
)

const modHead = "Module {\n    name: \"%s\"\n    kigumi: \"0.1\"\n}\n"

func manifest(name, rest string) []byte {
	return []byte(strings.Replace(modHead, "%s", name, 1) + rest)
}

func TestParseModFile(t *testing.T) {
	src := "Module { name: \"github.com/acme/app\", kigumi: \"0.1\" }\n\nRequire {\n    name: \"github.com/acme/greet\"\n    version: \"v0.1.0\"\n}\nLink { library: \"m\" }\nSource { c: \"bridge/x.c\" }\nHeader { path: \"vendor/x.h\", package: \"x\" }\n"
	mf, ds := driver.ParseModFile(token.NewFile("mod.kg", []byte(src)))
	if len(ds) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", ds)
	}
	if mf.Name != "github.com/acme/app" || mf.Kigumi != "0.1" || len(mf.Requires) != 1 || mf.Requires[0] != (driver.Require{Name: "github.com/acme/greet", Version: "v0.1.0"}) || len(mf.Links) != 1 || mf.Sources[0] != (driver.NativeSource{Path: "bridge/x.c"}) || mf.Headers[0] != (driver.HeaderImport{Path: "vendor/x.h", Package: "x"}) {
		t.Fatalf("parsed %+v", mf)
	}
	bad := map[string]string{
		"Module { name: \"a\" }\n":                                                                   "needs `kigumi`",
		"Module { name: \"a\", kigumi: \"9.0\" }\n":                                                  "needs kigumi 9.0",
		"Module { name: \"a\", kigumi: \"not-a-version\" }\n":                                        "is not a number",
		"Require { name: \"x\", version: \"v1.0.0\" }\n":                                             "needs a Module record",
		"Module { name: \"a\", kigumi: \"0.1\" }\nReplace { name: \"x\", path: \"y\" }\n":            "belongs in mod.local.kg",
		"Module { name: \"a\", kigumi: \"0.1\" }\nLink { library: \"/usr/lib/x\" }\n":                "is a name, not a path",
		"Module { name: \"a\", kigumi: \"0.1\" }\nLink { search: \"../elsewhere\" }\n":               "inside the module",
		"Module { name: \"a\", kigumi: \"0.1\" }\nSource { c: \"/abs/x.c\" }\n":                      "stay inside the module",
		"Module { name: \"a\", kigumi: \"0.1\" }\nSource { c: \"x.c\", asm: \"y.s\" }\n":             "exactly one of",
		"Module { name: \"a\", kigumi: \"0.1\" }\nHeader { path: \"x.h\" }\n":                        "needs both",
		"Module { name: \"a\", kigumi: \"0.1\" }\nHeader { path: \"/abs/x.h\", package: \"x\" }\n":   "file inside the module",
		"Module { name: \"a\", kigumi: \"0.1\" }\nHeader { path: \"x.h\", package: \"1x\" }\n":       "look like an identifier",
		"Module { name: \"a/../b\", kigumi: \"0.1\" }\n":                                             "has a `..` segment",
		"Module { name: \"a//b\", kigumi: \"0.1\" }\n":                                               "empty path segment",
		"Module { name: \"a b\", kigumi: \"0.1\" }\n":                                                "may only use letters",
		"Module { name: \"a\", kigumi: \"0.1\" }\nRequire { name: \"x\", version: \"1.0\" }\n":       "is not `v<major>.<minor>.<patch>",
		"Module { name: \"a\", kigumi: \"0.1\" }\nRequire { name: \"x\", version: \"v1.0\" }\n":      "is not `v<major>.<minor>.<patch>",
		"Module { name: \"a\", kigumi: \"0.1\" }\nRequire { name: \"x\", version: \"v1.0.0-01\" }\n": "leading zero",
		"Module { name: \"a\", kigumi: \"0.1\" }\nRequire { name: \"x@y\", version: \"v1.0.0\" }\n":  "may only use letters",
		"Dep { name: \"x\" }\n": "unknown record Dep",
		"Module { name: 3 }\n":  "plain string literal",
		"let x = 1\n":           "expected a record",
	}
	for src, want := range bad {
		_, ds := driver.ParseModFile(token.NewFile("mod.kg", []byte(src)))
		found := false
		for _, d := range ds {
			found = found || strings.Contains(d.Msg, want)
		}
		if !found {
			t.Errorf("%q: want %q, got %+v", src, want, ds)
		}
	}
}

func TestRepoOf(t *testing.T) {
	cases := []struct{ name, url, wantURL, wantSub string }{
		{"github.com/kigumilang/exp/sgx", "", "https://github.com/kigumilang/exp.git", "sgx"},
		{"github.com/hayao0819/greet", "", "https://github.com/hayao0819/greet.git", ""},
		{"example.com/team/mono.git/tools", "", "https://example.com/team/mono.git", "tools"},
		{"example.com/team/mono.git/tools", "ssh://git@example.com/team/mono.git", "ssh://git@example.com/team/mono.git", "tools"},
		{"github.com/kigumilang/exp/sgx", "/tmp/checkouts/github.com/kigumilang/exp", "/tmp/checkouts/github.com/kigumilang/exp", "sgx"},
		{"greet", "/tmp/greet.git", "/tmp/greet.git", ""},
	}
	for _, c := range cases {
		url, sub, ok := driver.RepoOf(driver.Require{Name: c.name, URL: c.url})
		if !ok || url != c.wantURL || sub != c.wantSub {
			t.Errorf("%s (%s): got %q %q %v, want %q %q", c.name, c.url, url, sub, ok, c.wantURL, c.wantSub)
		}
	}
	if _, _, ok := driver.RepoOf(driver.Require{Name: "greet"}); ok {
		t.Error("a short name has no derivable repo")
	}
	if driver.TagOf("sgx", "v0.1.0") != "sgx/v0.1.0" || driver.TagOf("", "v1.0.0") != "v1.0.0" || !driver.IsCommitHash("3f2c9a1") || driver.IsCommitHash("v0.1.0") {
		t.Error("tag or hash rules")
	}
}

func TestManifestEdit(t *testing.T) {
	root := t.TempDir()
	if err := driver.WriteManifest(root, "github.com/acme/app"); err != nil {
		t.Fatal(err)
	}
	if err := driver.WriteManifest(root, "github.com/acme/app"); err == nil {
		t.Fatal("second init should refuse to overwrite")
	}
	mf, _, err := driver.ReadModFile(root)
	if err != nil || mf.Kigumi != driver.ToolchainVersion {
		t.Fatalf("init should record the toolchain version: %+v %v", mf, err)
	}
	os.WriteFile(filepath.Join(root, "mod.kg"), manifest("app", "\n// keep me\nRequire {\n    name: \"a\"\n    version: \"v1.0.0\"\n    url: \"u\"\n}\n"), 0o644)
	if err := driver.AppendRequire(root, driver.Require{Name: "github.com/acme/b", Version: "v2.0.0"}); err != nil {
		t.Fatal(err)
	}
	if err := driver.AppendRequire(root, driver.Require{Name: "github.com/acme/b", Version: "v3.0.0"}); err == nil {
		t.Fatal("duplicate require should be refused")
	}
	if err := driver.AppendLocalReplace(root, "github.com/acme/b", "../b"); err != nil {
		t.Fatal(err)
	}
	local, err := driver.ReadLocal(filepath.Join(root, "deep", "er"))
	if err != nil || local.Replaces["github.com/acme/b"] != filepath.Join(root, "../b") {
		t.Fatalf("local override: %+v %v", local, err)
	}
	t.Chdir(root)
	if local, err = driver.ReadLocal(filepath.Join("deep", "er")); err != nil || local.Replaces["github.com/acme/b"] != filepath.Join(root, "../b") {
		t.Fatalf("local override from a relative dir: %+v %v", local, err)
	}
	if err := driver.RemoveRequires(root, map[string]bool{"a": true}); err != nil {
		t.Fatal(err)
	}
	src, _ := os.ReadFile(filepath.Join(root, "mod.kg"))
	if !strings.Contains(string(src), "// keep me") || strings.Contains(string(src), "\"a\"") || !strings.Contains(string(src), "acme/b") {
		t.Fatalf("remove kept the wrong lines:\n%s", src)
	}

	script := filepath.Join(root, "tool.kg")
	os.WriteFile(script, []byte("#!/usr/bin/env kigumi\n\n// a script\nimport {hello} from github.com/acme/greet\nprint hello()\n"), 0o644)
	if err := driver.AppendScriptRequire(script, driver.Require{Name: "github.com/acme/greet", Version: "v0.1.0"}); err != nil {
		t.Fatal(err)
	}
	if err := driver.AppendScriptRequire(script, driver.Require{Name: "github.com/acme/other", Version: "v0.2.0"}); err != nil {
		t.Fatal(err)
	}
	mf, has, err := driver.ReadScriptManifest(script)
	if err != nil || !has || mf.Kigumi != driver.ToolchainVersion || len(mf.Requires) != 2 || mf.Requires[1].Name != "github.com/acme/other" {
		t.Fatalf("script manifest: %+v %v %v", mf, has, err)
	}
	src, _ = os.ReadFile(script)
	if !strings.HasPrefix(string(src), "#!/usr/bin/env kigumi\n\n// a script\nKigumi { version:") || !strings.Contains(string(src), "}\nRequire {") {
		t.Fatalf("records should sit after the shebang and comments:\n%s", src)
	}
}

func TestLockRoundTrip(t *testing.T) {
	root := t.TempDir()
	lk := driver.Lock{Entries: map[string]driver.LockEntry{"b": {Version: "v2.0.0", Commit: "abc", Hash: "h1:1"}, "a": {Version: "v1.0.0", Hash: "h1:0"}}}
	if err := driver.WriteLock(root, lk); err != nil {
		t.Fatal(err)
	}
	got, err := driver.ReadLock(root)
	if err != nil || len(got.Entries) != 2 || got.Entries["b"] != lk.Entries["b"] || got.Entries["a"] != lk.Entries["a"] {
		t.Fatalf("read back %+v %v", got, err)
	}
	if err := driver.WriteLock(root, driver.Lock{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "mod.lock.kg")); !os.IsNotExist(err) {
		t.Fatal("empty lock should remove the file")
	}
	dir := filepath.Join(root, "tree")
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "x.kg"), []byte("a"), 0o644)
	h1, _ := driver.TreeHash(dir)
	os.WriteFile(filepath.Join(dir, "sub", "x.kg"), []byte("b"), 0o644)
	h2, _ := driver.TreeHash(dir)
	if h1 == h2 || !strings.HasPrefix(h1, "h1:") {
		t.Fatalf("tree hash should follow content: %s %s", h1, h2)
	}
}

// TestDependencyConflict requires lib at two versions through two paths
// (resolved by selection), rejects overlapping names, and checks the
// longest-match attribution of nested module names.
func TestDependencyConflict(t *testing.T) {
	root := t.TempDir()
	write := func(dir, name string, body []byte) {
		os.MkdirAll(filepath.Join(root, dir), 0o755)
		os.WriteFile(filepath.Join(root, dir, name), body, 0o644)
	}
	write("lib", "mod.kg", manifest("github.com/acme/lib", ""))
	write("lib", "lib.kg", []byte("pub fn one() -> Int {\n    1\n}\n"))
	write("mid", "mod.kg", manifest("github.com/acme/mid", "Require { name: \"github.com/acme/lib\", version: \"v1.1.0\" }\n"))
	write("mid", "mid.kg", []byte("pub fn two() -> Int {\n    2\n}\n"))
	write("app", "mod.kg", manifest("github.com/acme/app", "Require { name: \"github.com/acme/lib\", version: \"v1.0.0\" }\nRequire { name: \"github.com/acme/mid\", version: \"v0.1.0\" }\n"))
	write("app", "mod.local.kg", []byte("Replace { name: \"github.com/acme/lib\", path: \"../lib\" }\nReplace { name: \"github.com/acme/mid\", path: \"../mid\" }\n"))
	write("app", "main.kg", []byte("import {two} from github.com/acme/mid\nprint \"${two()}\"\n"))
	// Minimal version selection reconciles v1.0.0 (app) with v1.1.0 (mid);
	// a different major (no major suffix) is refused instead.
	m0, err := driver.LoadModule(filepath.Join(root, "app"), driver.LoadOptions{})
	if err != nil || m0.Selected["github.com/acme/lib"].Version != "v1.1.0" {
		t.Fatalf("selection: %v %+v", err, m0.Selected)
	}
	write("mid", "mod.kg", manifest("github.com/acme/mid", "Require { name: \"github.com/acme/lib\", version: \"v2.0.0\" }\n"))
	if _, err := driver.LoadModule(filepath.Join(root, "app"), driver.LoadOptions{}); err == nil || !strings.Contains(err.Error(), "different major versions cannot be mixed") {
		t.Fatalf("major mix not reported: %v", err)
	}
	// A required name nested under the root's own name is
	// only a naming detail: fetching aside, it must not be rejected as an
	// overlap by name alone.
	write("app", "mod.kg", manifest("github.com/acme/app", "Require { name: \"github.com/acme/app/sub\", version: \"v1.0.0\" }\n"))
	m, err := driver.LoadModule(filepath.Join(root, "app"), driver.LoadOptions{NoDeps: true})
	if err != nil {
		t.Fatal(err)
	}
	used := m.ImportedModules(driver.ModFile{Requires: []driver.Require{{Name: "github.com/acme"}, {Name: "github.com/acme/mid"}}})
	if !used["github.com/acme/mid"] || used["github.com/acme"] {
		t.Fatalf("imported modules should use the longest match: %v", used)
	}
	if _, ok := m.Packages["github.com/acme/app"]; !ok || m.Rel("github.com/acme/app/cmd/x") != "cmd/x" {
		t.Fatalf("own packages carry the module name: %v", m.Order)
	}
}

// TestModuleNamePrefixNotOverlap: a monorepo example nested
// under its own library depends on that library and loads and builds,
// while a dependency whose package path actually collides with one the
// root already provides is still rejected.
func TestModuleNamePrefixNotOverlap(t *testing.T) {
	root := t.TempDir()
	write := func(dir, name string, body []byte) {
		os.MkdirAll(filepath.Join(root, dir), 0o755)
		os.WriteFile(filepath.Join(root, dir, name), body, 0o644)
	}
	write("lib", "mod.kg", manifest("example.com/lib", ""))
	write("lib", "lib.kg", []byte("pub fn helper() -> Int {\n    1\n}\n"))
	demo := "lib/examples/demo"
	write(demo, "mod.kg", manifest("example.com/lib/examples/demo", "Require { name: \"example.com/lib\", version: \"v1.0.0\" }\n"))
	write(demo, "mod.local.kg", []byte("Replace { name: \"example.com/lib\", path: \"../..\" }\n"))
	write(demo, "main.kg", []byte("import {helper} from example.com/lib\nprint \"${helper()}\"\n"))
	m, err := driver.LoadModule(filepath.Join(root, demo), driver.LoadOptions{})
	if err != nil {
		t.Fatalf("example nested under its own library should load: %v", err)
	}
	if _, ok := m.Packages["example.com/lib"]; !ok {
		t.Fatalf("dependency package missing: %v", m.Order)
	}
	var out, errOut strings.Builder
	if code, err := driver.Run(m, &out, &errOut, []string{"app"}); err != nil || code != 0 || out.String() != "1\n" {
		t.Fatalf("run: %v exit %d out %q err %q", err, code, out.String(), errOut.String())
	}

	write("dup", "mod.kg", manifest("example.com/dup", "Require { name: \"example.com/dup/sub\", version: \"v1.0.0\" }\n"))
	write("dup/sub", "sub.kg", []byte("pub fn x() -> Int {\n    1\n}\n"))
	write("dup", "mod.local.kg", []byte("Replace { name: \"example.com/dup/sub\", path: \"../othersub\" }\n"))
	write("othersub", "othersub.kg", []byte("pub fn y() -> Int {\n    2\n}\n"))
	if _, err := driver.LoadModule(filepath.Join(root, "dup"), driver.LoadOptions{}); err == nil || !strings.Contains(err.Error(), "overlaps this module's name") {
		t.Fatalf("real overlap not reported: %v", err)
	}
}

// TestDependencyEmptyRootNotOverlap covers a false-positive edge case:
// a dependency whose own root directory has no package file of its own
// (all its code lives in a subdirectory) must not be rejected merely
// because its bare name coincides with a package path the root already
// owns, since loading it would never actually produce a duplicate.
func TestDependencyEmptyRootNotOverlap(t *testing.T) {
	root := t.TempDir()
	write := func(dir, name string, body []byte) {
		os.MkdirAll(filepath.Join(root, dir), 0o755)
		os.WriteFile(filepath.Join(root, dir, name), body, 0o644)
	}
	write("lib", "mod.kg", manifest("example.com/lib", "Require { name: \"example.com/lib/app\", version: \"v1.0.0\" }\n"))
	write("lib", "mod.local.kg", []byte("Replace { name: \"example.com/lib/app\", path: \"../dep\" }\n"))
	write("lib/app", "app.kg", []byte("pub fn helper() -> Int {\n    1\n}\n"))
	write("lib", "main.kg", []byte("import {deep} from example.com/lib/app/deep\nprint \"${deep()}\"\n"))
	write("dep", "mod.kg", manifest("example.com/lib/app", ""))
	write("dep/deep", "deep.kg", []byte("pub fn deep() -> Int {\n    2\n}\n"))
	m, err := driver.LoadModule(filepath.Join(root, "lib"), driver.LoadOptions{})
	if err != nil {
		t.Fatalf("dependency with an empty root directory should not overlap: %v", err)
	}
	if _, ok := m.Packages["example.com/lib/app"]; !ok {
		t.Fatalf("root's own app package missing: %v", m.Order)
	}
	if _, ok := m.Packages["example.com/lib/app/deep"]; !ok {
		t.Fatalf("dependency's deep package missing: %v", m.Order)
	}
	var out, errOut strings.Builder
	if code, err := driver.Run(m, &out, &errOut, []string{"lib"}); err != nil || code != 0 || out.String() != "2\n" {
		t.Fatalf("run: %v exit %d out %q err %q", err, code, out.String(), errOut.String())
	}
}

// TestVersionSelection picks the highest version any path asks for and
// reads the manifests of the selected versions only.
func TestVersionSelection(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	lock := driver.Lock{Entries: map[string]driver.LockEntry{}}
	put := func(name, version, rest string, files map[string]string) {
		dir, _ := driver.CacheDir(driver.Require{Name: name, Version: version})
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "mod.kg"), manifest(name, rest), 0o644)
		os.WriteFile(filepath.Join(dir, driver.OriginFileName), []byte("cafebabe\n"), 0o644)
		for f, body := range files {
			os.WriteFile(filepath.Join(dir, f), []byte(body), 0o644)
		}
		hash, err := driver.TreeHash(dir)
		if err != nil {
			t.Fatal(err)
		}
		lock.Entries[name] = driver.LockEntry{Version: version, Commit: "cafebabe", Hash: hash}
	}
	put("github.com/acme/lib", "v1.0.0", "", map[string]string{"lib.kg": "pub fn one() -> Int {\n    1\n}\n"})
	put("github.com/acme/lib", "v1.2.0", "", map[string]string{"lib.kg": "pub fn one() -> Int {\n    2\n}\n"})
	put("github.com/acme/mid", "v0.1.0", "Require { name: \"github.com/acme/lib\", version: \"v1.2.0\" }\n", map[string]string{"mid.kg": "import {one} from github.com/acme/lib\n\npub fn two() -> Int {\n    one() * 10\n}\n"})
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), manifest("app", "Require { name: \"github.com/acme/lib\", version: \"v1.0.0\" }\nRequire { name: \"github.com/acme/mid\", version: \"v0.1.0\" }\n"), 0o644)
	if err := driver.WriteLock(root, lock); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("import {one} from github.com/acme/lib\nimport {two} from github.com/acme/mid\nprint \"${one()} ${two()}\"\n"), 0o644)
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	if m.Selected["github.com/acme/lib"].Version != "v1.2.0" {
		t.Fatalf("selection: %+v", m.Selected)
	}
	var out, errOut strings.Builder
	if code, err := driver.Run(m, &out, &errOut, []string{"app"}); err != nil || code != 0 || out.String() != "2 20\n" {
		t.Fatalf("run: %v exit %d out %q err %q", err, code, out.String(), errOut.String())
	}
	if c := driver.CompareVersions("v1.10.0", "v1.9.9"); c <= 0 {
		t.Errorf("numeric compare: %d", c)
	}
	if c := driver.CompareVersions("v2.0.0-rc1", "v2.0.0"); c >= 0 {
		t.Errorf("prerelease should sort first: %d", c)
	}
	put("github.com/acme/lib", "abc1234", "", map[string]string{"lib.kg": "pub fn one() -> Int {\n    3\n}\n"})
	os.WriteFile(filepath.Join(root, "mod.kg"), manifest("app", "Require { name: \"github.com/acme/lib\", version: \"abc1234\" }\nRequire { name: \"github.com/acme/mid\", version: \"v0.1.0\" }\n"), 0o644)
	if _, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std}); err == nil || !strings.Contains(err.Error(), "commit hash cannot be reconciled") {
		t.Fatalf("hash versus tag should be an error: %v", err)
	}
}
