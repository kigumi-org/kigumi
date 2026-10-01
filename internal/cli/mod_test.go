package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const modHead = "Module {\n    name: \"%s\"\n    kigumi: \"0.1\"\n}\n"

func manifest(name, rest string) []byte {
	return []byte(strings.Replace(modHead, "%s", name, 1) + rest)
}

func git(t *testing.T, dir string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("git unavailable: %v %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// monorepo builds a repo whose checkout path ends in the module's host
// path, so a Require can point its url at it.
func monorepo(t *testing.T, root string) string {
	repo := filepath.Join(root, "github.com", "kigumilang", "exp")
	os.MkdirAll(filepath.Join(repo, "sgx", "enclave"), 0o755)
	os.MkdirAll(filepath.Join(repo, "web"), 0o755)
	os.WriteFile(filepath.Join(repo, "sgx", "mod.kg"), manifest("github.com/kigumilang/exp/sgx", ""), 0o644)
	os.WriteFile(filepath.Join(repo, "sgx", "enclave", "enclave.kg"), []byte("pub fn hello() -> String {\n    \"hi\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "web", "mod.kg"), manifest("github.com/kigumilang/exp/web", ""), 0o644)
	os.WriteFile(filepath.Join(repo, "web", "web.kg"), []byte("pub fn serve() -> Int {\n    1\n}\n"), 0o644)
	git(t, repo, "init", "-q")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "initial")
	git(t, repo, "tag", "sgx/v0.1.0")
	git(t, repo, "tag", "web/v0.3.0")
	return repo
}

// TestGetLock fetches a nested module by its tag, checks the lock, catches
// a tampered cache and a moved tag, and fetches a commit hash.
func TestGetLock(t *testing.T) {
	root := t.TempDir()
	repo := monorepo(t, root)
	app := filepath.Join(root, "app")
	os.MkdirAll(app, 0o755)
	os.WriteFile(filepath.Join(app, "mod.kg"), manifest("app", "Require {\n    name: \"github.com/kigumilang/exp/sgx\"\n    version: \"v0.1.0\"\n    url: \""+repo+"\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(app, "main.kg"), []byte("import enclave from github.com/kigumilang/exp/sgx/enclave\nprint enclave.hello()\n"), 0o644)
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	if code, out, errOut := execute(t, "get", app); code != 0 {
		t.Fatalf("get: %s%s", out, errOut)
	}
	cached := filepath.Join(cache, "kigumi", "mod", "github.com", "kigumilang", "exp", "sgx@v0.1.0")
	if _, err := os.Stat(filepath.Join(cached, "enclave", "enclave.kg")); err != nil {
		t.Fatalf("subtree not extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cached, "..", "web")); err == nil {
		t.Fatal("only the module's subdirectory belongs in the cache")
	}
	lock, err := os.ReadFile(filepath.Join(app, "mod.lock.kg"))
	if err != nil || !strings.Contains(string(lock), "name: \"github.com/kigumilang/exp/sgx\"") {
		t.Fatalf("lock not written: %v\n%s", err, lock)
	}
	std, _ := filepath.Abs("../../std")
	if code, out, errOut := execute(t, "--std", std, "run", app); code != 0 || out != "hi\n" {
		t.Fatalf("run: exit %d out %q err %q", code, out, errOut)
	}
	if code, out, _ := execute(t, "mod", "verify", "-C", app); code != 0 || !strings.Contains(out, "ok") {
		t.Fatalf("verify: exit %d %q", code, out)
	}

	os.WriteFile(filepath.Join(cached, "enclave", "enclave.kg"), []byte("pub fn hello() -> String {\n    \"tampered\"\n}\n"), 0o644)
	if code, out, _ := execute(t, "mod", "verify", "-C", app); code != 1 || !strings.Contains(out, "MISMATCH") {
		t.Fatalf("verify after tampering: exit %d %q", code, out)
	}
	if code, _, _ := execute(t, "get", app); code == 0 {
		t.Fatal("get should reject a cache that differs from the lock")
	}

	os.RemoveAll(cached)
	os.WriteFile(filepath.Join(repo, "sgx", "enclave", "enclave.kg"), []byte("pub fn hello() -> String {\n    \"moved\"\n}\n"), 0o644)
	git(t, repo, "commit", "-q", "-am", "move tag")
	git(t, repo, "tag", "-f", "sgx/v0.1.0")
	if code, _, _ := execute(t, "get", app); code == 0 {
		t.Fatal("get should notice the tag moved")
	}
	if code, _, errOut := execute(t, "get", "-u", app); code != 0 {
		t.Fatalf("get -u: %s", errOut)
	}
	if code, out, _ := execute(t, "--std", std, "run", app); code != 0 || out != "moved\n" {
		t.Fatalf("run after -u: exit %d out %q", code, out)
	}

	hash := git(t, repo, "rev-parse", "--short=9", "HEAD")
	os.WriteFile(filepath.Join(app, "mod.kg"), manifest("app", "Require {\n    name: \"github.com/kigumilang/exp/sgx\"\n    version: \""+hash+"\"\n    url: \""+repo+"\"\n}\n"), 0o644)
	if code, _, errOut := execute(t, "get", app); code != 0 {
		t.Fatalf("get by commit: %s", errOut)
	}
	lock, _ = os.ReadFile(filepath.Join(app, "mod.lock.kg"))
	if !strings.Contains(string(lock), "version: \""+hash+"\"") || strings.Contains(string(lock), "commit: \""+hash+"\"") {
		t.Fatalf("lock should keep the short version and the full commit:\n%s", lock)
	}
	if code, out, _ := execute(t, "--std", std, "run", app); code != 0 || out != "moved\n" {
		t.Fatalf("run by commit: exit %d out %q", code, out)
	}
}

// TestGetAdd resolves the newest tag of a module path and appends the
// Require; a script gets the records at its top.
func TestGetAdd(t *testing.T) {
	root := t.TempDir()
	repo := monorepo(t, root)
	git(t, repo, "tag", "web/v0.4.0-rc1")
	app := filepath.Join(root, "app")
	os.MkdirAll(app, 0o755)
	os.WriteFile(filepath.Join(app, "mod.kg"), manifest("app", ""), 0o644)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if code, out, errOut := execute(t, "get", "github.com/kigumilang/exp/web", "--url", repo, app); code != 0 || !strings.Contains(out, "added github.com/kigumilang/exp/web v0.3.0") {
		t.Fatalf("get add: exit %d %q %q", code, out, errOut)
	}
	mf, _ := os.ReadFile(filepath.Join(app, "mod.kg"))
	if !strings.Contains(string(mf), "version: \"v0.3.0\"") {
		t.Fatalf("manifest after add:\n%s", mf)
	}

	script := filepath.Join(root, "tool.kg")
	os.WriteFile(script, []byte("#!/usr/bin/env kigumi\nimport web from github.com/kigumilang/exp/web\nprint \"${web.serve()}\"\n"), 0o644)
	if code, _, errOut := execute(t, "get", "github.com/kigumilang/exp/web@v0.3.0", "--url", repo, "--lock", script); code != 0 {
		t.Fatalf("get add script: %s", errOut)
	}
	src, _ := os.ReadFile(script)
	if !strings.HasPrefix(string(src), "#!/usr/bin/env kigumi\nKigumi { version:") {
		t.Fatalf("script records:\n%s", src)
	}
	if _, err := os.Stat(script + ".lock.kg"); err != nil {
		t.Fatal("--lock should write the script's lock")
	}
	std, _ := filepath.Abs("../../std")
	if code, out, errOut := execute(t, "--std", std, "run", script); code != 0 || out != "1\n" {
		t.Fatalf("run script: exit %d out %q err %q", code, out, errOut)
	}
}

func TestModCommands(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "lib")
	os.MkdirAll(lib, 0o755)
	os.WriteFile(filepath.Join(lib, "mod.kg"), manifest("github.com/acme/lib", ""), 0o644)
	os.WriteFile(filepath.Join(lib, "lib.kg"), []byte("pub fn one() -> Int {\n    1\n}\n"), 0o644)
	app := filepath.Join(root, "app")
	os.MkdirAll(app, 0o755)
	if code, out, _ := execute(t, "mod", "init", "-C", app, "github.com/acme/app"); code != 0 || !strings.Contains(out, "mod.kg") {
		t.Fatalf("init: exit %d %q", code, out)
	}
	if code, _, _ := execute(t, "mod", "init", "-C", app); code != 2 {
		t.Fatalf("second init should be a usage error, exit %d", code)
	}
	if code, _, _ := execute(t, "mod", "add", "-C", app, "lib", "v1.0.0"); code != 2 {
		t.Fatalf("add of a short name without --url should be a usage error, exit %d", code)
	}
	if code, _, errOut := execute(t, "mod", "add", "-C", app, "github.com/acme/lib", "v1.0.0"); code != 0 {
		t.Fatalf("add: %s", errOut)
	}
	if code, _, errOut := execute(t, "mod", "replace", "-C", app, "github.com/acme/lib", "../lib"); code != 0 {
		t.Fatalf("replace: %s", errOut)
	}
	if code, _, errOut := execute(t, "mod", "add", "-C", app, "github.com/acme/unused", "v2.0.0"); code != 0 {
		t.Fatalf("add: %s", errOut)
	}
	mf, _ := os.ReadFile(filepath.Join(app, "mod.kg"))
	if !strings.Contains(string(mf), "kigumi: \"0.1\"") || strings.Contains(string(mf), "Replace") || !strings.Contains(string(mf), "acme/unused") {
		t.Fatalf("manifest after add:\n%s", mf)
	}
	if local, err := os.ReadFile(filepath.Join(app, "mod.local.kg")); err != nil || !strings.Contains(string(local), "path: \"../lib\"") {
		t.Fatalf("local override: %v\n%s", err, local)
	}
	os.WriteFile(filepath.Join(app, "main.kg"), []byte("import {one} from github.com/acme/lib\nprint \"${one()}\"\n"), 0o644)
	if code, out, errOut := execute(t, "mod", "tidy", "-C", app); code != 0 || !strings.Contains(out, "removed github.com/acme/unused") {
		t.Fatalf("tidy: exit %d %q %q", code, out, errOut)
	}
	if code, out, _ := execute(t, "mod", "json", "-C", app); code != 0 || !strings.Contains(out, "\"local\"") || !strings.Contains(out, "acme/lib") {
		t.Fatalf("json: exit %d %q", code, out)
	}
	std, _ := filepath.Abs("../../std")
	if code, out, errOut := execute(t, "--std", std, "run", app); code != 0 || out != "1\n" {
		t.Fatalf("run: exit %d out %q err %q", code, out, errOut)
	}
	if code, _, _ := execute(t, "--std", std, "--no-local", "run", app); code == 0 {
		t.Fatal("--no-local must ignore the override and miss the dependency")
	}
	if code, out, _ := execute(t, "--version"); code != 0 || !strings.HasPrefix(out, "kigumi version ") {
		t.Fatalf("version: exit %d %q", code, out)
	}
}

// TestSchema lists declarations with their metadata attributes.
func TestSchema(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "web"), 0o755)
	os.WriteFile(filepath.Join(root, "mod.kg"), manifest("app", ""), 0o644)
	os.WriteFile(filepath.Join(root, "web", "web.kg"), []byte("pub type Metadata = {\n    tag String\n}\n\npub pure fn get(path: String) -> Metadata {\n    Metadata { tag: path }\n}\n\npub pure fn param(name: String) -> Metadata {\n    Metadata { tag: name }\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("import web from app/web\n\n@web.get(\"/users/:id\")\nfn show(@web.param(\"id\") id: Int) -> Int {\n    id\n}\n\nfn helper() -> Int {\n    1\n}\n\nprint \"${show(1)} ${helper()}\"\n"), 0o644)
	std, _ := filepath.Abs("../../std")
	code, out, errOut := execute(t, "--std", std, "schema", "--attr", "web.get", root)
	if code != 0 {
		t.Fatalf("schema: %s", errOut)
	}
	for _, want := range []string{"\"name\": \"show\"", "\"name\": \"web.get\"", "\"/users/:id\"", "\"name\": \"web.param\""} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "helper") {
		t.Errorf("--attr should filter out undecorated declarations:\n%s", out)
	}
}
