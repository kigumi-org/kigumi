package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"kigumi/internal/driver"
	"kigumi/internal/llgen"
)

// TestPlatformFiles loads only the files whose suffix names the host.
func TestPlatformFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	other := "windows"
	if runtime.GOOS == "windows" {
		other = "linux"
	}
	body := func(s string) []byte { return []byte("pub fn name() -> String {\n    \"" + s + "\"\n}\n") }
	os.MkdirAll(filepath.Join(root, "sys"), 0o755)
	os.WriteFile(filepath.Join(root, "sys", "sys_"+runtime.GOOS+".kg"), body("host"), 0o644)
	os.WriteFile(filepath.Join(root, "sys", "sys_"+other+".kg"), body("other"), 0o644)
	os.WriteFile(filepath.Join(root, "sys", "sys_"+other+"_"+runtime.GOARCH+".kg"), body("other-arch"), 0o644)
	os.WriteFile(filepath.Join(root, "sys", "arch_"+runtime.GOARCH+".kg"), []byte("pub fn arch() -> String {\n    \""+runtime.GOARCH+"\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"plat\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("import sys from plat/sys\nprint \"${sys.name()} ${sys.arch()}\"\n"), 0o644)
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(m.Packages["plat/sys"].Files); n != 2 {
		t.Fatalf("expected the host file and the arch file, got %d files", n)
	}
	var out, errOut bytes.Buffer
	if code, err := driver.Run(m, &out, &errOut, []string{"plat"}); err != nil || code != 0 || out.String() != "host "+runtime.GOARCH+"\n" {
		t.Fatalf("run: %v exit %d out %q err %q", err, code, out.String(), errOut.String())
	}
}

// TestComptimeSandbox rejects host effects inside comptime blocks.
func TestComptimeSandbox(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"ct\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("import random from std/random\nlet n = comptime {\n    let mut r = random.fromClock()\n    r.nextInt(6)\n}\nprint \"${n}\"\n"), 0o644)
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	if out := res.Render(); !strings.Contains(out, "not available at compile time") {
		t.Fatalf("expected a sandbox error, got:\n%s", out)
	}
}

// TestArenaAllocates checks the native runtime really routes allocations
// through an arena; the interpreter reports 0 so this cannot be a shared
// fixture.
func TestArenaAllocates(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root := t.TempDir()
	src := "import {Arena} from std/alloc\n\nfn run() -> Unit! {\n    let arena = Arena.create()?\n    let before = arena.allocated()\n    let n = allocator arena.scope() {\n        let mut xs = Array.empty[Int]()\n        for i in 0..1000 {\n            xs.push(i)\n        }\n        xs.len()\n    }\n    let after = arena.allocated()\n    print \"${n} ${before == 0} ${after > 8000}\"\n}\n\nlet r = run()\n"
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"arena\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "prog")
	var buildErr bytes.Buffer
	if ok, err := testBuild(m, exe, &buildErr); err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}
	out, err := exec.Command(exe).CombinedOutput()
	if err != nil || string(out) != "1000 true true\n" {
		t.Fatalf("got %q (%v)", out, err)
	}
}

// TestBuildFreestanding produces an archive with a kigumi_main entry and no
// POSIX layer, and selects platform files by the requested target.
func TestBuildFreestanding(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"bare\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("import greet from bare/greet\nprint greet.hello()\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "greet"), 0o755)
	os.WriteFile(filepath.Join(root, "greet", "greet_freestanding.kg"), []byte("pub fn hello() -> String {\n    \"bare metal\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "greet", "greet_linux.kg"), []byte("pub fn hello() -> String {\n    \"linux\"\n}\n"), 0o644)
	target, err := driver.ParseTarget("x86_64-freestanding", "")
	if err != nil || !target.Freestanding() || target.Sys != "none" || target.OS != "freestanding" || target.Arch != "amd64" {
		t.Fatalf("target: %+v %v", target, err)
	}
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(m.Packages["bare/greet"].Files); n != 1 || m.Packages["bare/greet"].Files[0].File.Name != "greet/greet_freestanding.kg" {
		t.Fatalf("platform files should follow the target: %v", m.Packages["bare/greet"].Files)
	}
	out := filepath.Join(root, "bare.a")
	var buildErr bytes.Buffer
	if ok, err := driver.BuildWith(m, out, &buildErr, driver.BuildOptions{Target: target}); err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil || !bytes.HasPrefix(data, []byte("!<arch>")) {
		t.Fatalf("expected a static archive: %v", err)
	}
	if !bytes.Contains(data, []byte("kigumi_main")) || bytes.Contains(data, []byte("rt_plan_word")) {
		t.Fatal("archive should carry kigumi_main and the freestanding layer")
	}
	for _, bad := range []string{"fopen", "getaddrinfo", "fork"} {
		if bytes.Contains(data, []byte(bad)) {
			t.Errorf("freestanding archive must not reference %s", bad)
		}
	}
	if _, err := driver.ParseTarget("sparc-linux", ""); err == nil {
		t.Error("unknown architectures should be rejected")
	}
}

// TestBuildFreestandingNoCloseSymbol: rt_core.c's net
// accelerator used to call close(2) directly, so even a program that never
// touches std/net left an undefined `close` in a freestanding archive (an
// SGX enclave link fails on it, since no descriptor can exist there). nm -u
// checks the exact undefined-symbol name rather than a substring, since
// "close" is also a substring of File.close and rt_sys_close.
func TestBuildFreestandingNoCloseSymbol(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	nmPath, err := exec.LookPath("nm")
	if err != nil {
		t.Skip("no nm")
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"bare\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("print \"hi\"\n"), 0o644)
	target, err := driver.ParseTarget("x86_64-freestanding", "")
	if err != nil {
		t.Fatal(err)
	}
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "bare.a")
	var buildErr bytes.Buffer
	if ok, err := driver.BuildWith(m, out, &buildErr, driver.BuildOptions{Target: target}); err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}
	nmOut, err := exec.Command(nmPath, "-u", out).CombinedOutput()
	if err != nil {
		t.Fatalf("nm: %v\n%s", err, nmOut)
	}
	for _, line := range strings.Split(string(nmOut), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[len(fields)-1] == "close" {
			t.Fatalf("freestanding archive has an undefined close(2): %s", line)
		}
	}
}

// TestBuildFreestandingSymbolSet: nothing
// declared which symbols a `--sys none` archive may leave for the embedder,
// so a new runtime libc call (the close(2) case above) could silently
// break one. This builds an archive from a program importing a broad set
// of std packages, computes the symbols left undefined after the archive's
// own objects resolve each other (rt_core.c calling into rt_sys.c, say),
// and checks every one is in internal/llgen/runtime/freestanding_symbols.txt.
func TestBuildFreestandingSymbolSet(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	nmPath, err := exec.LookPath("nm")
	if err != nil {
		t.Skip("no nm")
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"symset\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	src := "import json from std/json\nimport map from std/map\nimport text from std/text\n" +
		"import math from std/math\n\ntype Report = {\n    pub total Int\n    pub ratio Float\n" +
		"    pub tags map.Map[String, Int]\n}\n\n" +
		"let src = \"{\\\"total\\\": 3, \\\"ratio\\\": 0.5, \\\"tags\\\": {\\\"x\\\": 1}}\"\n" +
		"let report = json.decode[Report](src)?\n" +
		"print \"roundtrip=${json.encode(report)?}\"\n" +
		"let f = text.parseFloat(\"3.14\") || 0.0\n" +
		"let r = math.sqrt(2.0) + math.pow(2.0, 3.0) + math.floor(1.5) + math.ceil(1.5) + math.round(1.5)\n" +
		"print \"${f} ${r}\"\n"
	os.WriteFile(filepath.Join(root, "main.kg"), []byte(src), 0o644)
	target, err := driver.ParseTarget("x86_64-freestanding", "")
	if err != nil {
		t.Fatal(err)
	}
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "symset.a")
	var buildErr bytes.Buffer
	if ok, err := driver.BuildWith(m, out, &buildErr, driver.BuildOptions{Target: target}); err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}
	nmOut, err := exec.Command(nmPath, "--format=posix", out).CombinedOutput()
	if err != nil {
		t.Fatalf("nm: %v\n%s", err, nmOut)
	}
	undefined, defined := map[string]bool{}, map[string]bool{}
	for _, line := range strings.Split(string(nmOut), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || len(fields[1]) != 1 {
			continue
		}
		name, typ := fields[0], fields[1][0]
		if typ == 'U' || typ == 'w' {
			undefined[name] = true
		} else {
			defined[name] = true
		}
	}
	allowed := llgen.FreestandingSymbols()
	for name := range undefined {
		if defined[name] || allowed[name] {
			continue
		}
		t.Errorf("freestanding archive requires undefined symbol %q, missing from internal/llgen/runtime/freestanding_symbols.txt; add it there if it is an intentional libc dependency, or implement it in rt_core.c/rt_sys.c otherwise", name)
	}
}

// TestBuildHostBare: `--sys none` on a
// hosted OS builds a `_bare.kg`-selected
// archive with a kigumi_main entry, like a freestanding target, but still
// against the real x86_64-linux headers rather than the musl substitution
// (Target.compileTriple stays keyed on Freestanding alone).
func TestBuildHostBare(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"hostbare\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte("import greet from hostbare/greet\nprint greet.hello()\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "greet"), 0o755)
	os.WriteFile(filepath.Join(root, "greet", "greet_bare.kg"), []byte("pub fn hello() -> String {\n    \"bare on linux\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "greet", "greet_linux.kg"), []byte("pub fn hello() -> String {\n    \"linux\"\n}\n"), 0o644)
	target, err := driver.ParseTarget("x86_64-linux", "none")
	if err != nil {
		t.Fatal(err)
	}
	if target.Freestanding() || !target.Bare() || target.OS != "linux" {
		t.Fatalf("target: %+v", target)
	}
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(m.Packages["hostbare/greet"].Files); n != 1 || m.Packages["hostbare/greet"].Files[0].File.Name != "greet/greet_bare.kg" {
		t.Fatalf("--sys none should select the bare platform file: %v", m.Packages["hostbare/greet"].Files)
	}
	out := filepath.Join(root, "libhostbare.a")
	var buildErr bytes.Buffer
	if ok, err := driver.BuildWith(m, out, &buildErr, driver.BuildOptions{Target: target}); err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil || !bytes.HasPrefix(data, []byte("!<arch>")) {
		t.Fatalf("expected a static archive: %v", err)
	}
	if !bytes.Contains(data, []byte("kigumi_main")) {
		t.Fatal("archive should carry kigumi_main")
	}
}

// TestBuildHostBareRejectsPlatform checks a program importing std/time
// under the host+`--sys none` profile fails to compile, so the SIM fails
// like real hardware (no clock), same as request 8's freestanding case.
func TestBuildHostBareRejectsPlatform(t *testing.T) {
	t.Parallel()
	root := writePlatformModule(t, "hostbaretime")
	target, err := driver.ParseTarget("x86_64-linux", "none")
	if err != nil {
		t.Fatal(err)
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

// TestBuildHostedUnchanged checks a plain hosted build (no --sys none)
// still links a normal executable that runs std/time, not an archive.
func TestBuildHostedUnchanged(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root := t.TempDir()
	src := "import time from std/time\nlet t = time.now()\nprint \"${t.unixMillis() > 0}\"\n"
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"hosted\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.kg"), []byte(src), 0o644)
	std, _ := filepath.Abs("../../std")
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "prog")
	var buildErr bytes.Buffer
	if ok, err := driver.BuildWith(m, exe, &buildErr, driver.BuildOptions{}); err != nil || !ok {
		t.Fatalf("build: %v\n%s", err, buildErr.String())
	}
	data, err := os.ReadFile(exe)
	if err != nil || bytes.HasPrefix(data, []byte("!<arch>")) {
		t.Fatal("a plain hosted build must not be a static archive")
	}
	out, err := exec.Command(exe).CombinedOutput()
	if err != nil || string(out) != "true\n" {
		t.Fatalf("got %q (%v)", out, err)
	}
}
