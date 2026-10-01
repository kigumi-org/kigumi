package driver_test

import (
	"bytes"
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/buildgraph"
	"kigumi/internal/driver"
)

// TestBuildProgramLink covers request 7(a): Artifact.linkerScript,
// linkFlags and entry reach the actual link command. The target is an
// explicit musl triple (zig's own bundled musl, not the host's glibc) so
// -static-pie produces a real static binary anywhere zig runs.
func TestBuildProgramLink(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root, out := t.TempDir(), t.TempDir()
	write := func(rel, body string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755)
		os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644)
	}
	write("mod.kg", "Module {\n    name: \"lk\"\n    kigumi: \"0.1\"\n}\n")
	write("cmd/app/main.kg", "print \"hi\"\n")
	// INSERT AFTER augments the default script with one extra section
	// instead of replacing it, so crt/libc startup still links normally
	// (ld and lld both support this form).
	write("extra.ld", "SECTIONS\n{\n  .kigumi_test (INFO) : { LONG(0x1234) }\n}\nINSERT AFTER .text\n")
	program := "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n" +
		"    let a = b.executable(\"app\", \"cmd/app\", b.target(\"x86_64-linux-musl\"))?\n" +
		"    let script = b.file(\"extra.ld\")?\n" +
		"    a.linkerScript(script)\n" +
		"    a.linkFlags(Array.of(\"-static-pie\"))\n" +
		"    a.entry(\"_start\")\n" +
		"}\n"
	os.WriteFile(filepath.Join(root, "build.kg"), []byte(program), 0o644)
	std, _ := filepath.Abs("../../std")
	p, _, err := driver.LoadBuildProgram(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	g, err := p.Run(driver.HostTarget(), nil, out)
	if err != nil {
		t.Fatal(err)
	}
	rg, err := driver.ResolveGraph(g, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan := driver.PlanJSON(rg)
	for _, want := range []string{"\"linkFlags\"", "-static-pie", "\"entry\": \"_start\"", "linkerScript"} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan lacks %s:\n%s", want, plan)
		}
	}
	var stderr bytes.Buffer
	if err := driver.ExecuteGraph(p, rg, out, driver.ExecOptions{}, &stderr); err != nil {
		t.Fatalf("execute: %v\n%s", err, stderr.String())
	}
	exe := filepath.Join(out, "app")
	got, err := exec.Command(exe).Output()
	if err != nil || string(got) != "hi\n" {
		t.Fatalf("app: %q %v", got, err)
	}
	f, err := elf.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, ph := range f.Progs {
		if ph.Type == elf.PT_INTERP {
			t.Error("app has a PT_INTERP segment; -static-pie did not take effect")
		}
	}
	if f.Section(".kigumi_test") == nil {
		t.Error("app has no .kigumi_test section; the linker script was not applied")
	}
}

// TestLinkOptionsNeedHostedTarget covers the resolve-time guard: a linker
// script, link flags or an entry symbol need a hosted target, since a bare
// target links as an archive (buildArchive), not through BuildWith's link.
func TestLinkOptionsNeedHostedTarget(t *testing.T) {
	t.Parallel()
	script := 0
	g := &buildgraph.Graph{Artifacts: []*buildgraph.Artifact{{
		ID: 0, Name: "x", Kind: "executable",
		Target:       buildgraph.Target{Triple: "x86_64-freestanding"},
		LinkerScript: &script,
	}}}
	if _, err := driver.ResolveGraph(g, nil); err == nil || !strings.Contains(err.Error(), "hosted target") {
		t.Fatalf("bare target with a linker script should be rejected: %v", err)
	}
	g.Artifacts[0].LinkerScript = nil
	g.Artifacts[0].Entry = "_start"
	if _, err := driver.ResolveGraph(g, nil); err == nil || !strings.Contains(err.Error(), "hosted target") {
		t.Fatalf("bare target with an entry symbol should be rejected: %v", err)
	}
}
