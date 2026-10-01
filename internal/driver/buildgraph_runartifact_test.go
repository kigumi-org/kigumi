package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/buildgraph"
	"kigumi/internal/driver"
)

// TestBuildProgramRunArtifact covers request 7(b): Builder.runArtifact runs
// a host executable built earlier in the same graph, under the same
// trusted-tool env allowlist as runTool.
func TestBuildProgramRunArtifact(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root, out := t.TempDir(), t.TempDir()
	write := func(rel, body string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755)
		os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644)
	}
	write("mod.kg", "Module {\n    name: \"ra\"\n    kigumi: \"0.1\"\n}\n")
	write("cmd/writer/main.kg", "import fs from std/fs\n\n"+
		"let out = host.args().get(1) || \"missing\"\n"+
		"host.files().writeText(fs.Path.fromString(out)?, \"ran\\n\")?\n")
	program := "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n" +
		"    let tool = b.executable(\"writer\", \"cmd/writer\", b.request().target())?\n" +
		"    let o = b.runArtifact(tool, Array.of(\"$out:0\"), Array.empty[build.FilePath](), Array.of(\"result.txt\"))?\n" +
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
	if strings.Join(rg.Order, " ") != "a0 t0" {
		t.Fatalf("order: %v", rg.Order)
	}
	plan := driver.PlanJSON(rg)
	if !strings.Contains(plan, "\"kind\": \"artifact\"") {
		t.Errorf("plan lacks the artifact tool step:\n%s", plan)
	}
	var stderr bytes.Buffer
	if err := driver.ExecuteGraph(p, rg, out, driver.ExecOptions{}, &stderr); err != nil {
		t.Fatalf("execute: %v\n%s", err, stderr.String())
	}
	got, err := os.ReadFile(filepath.Join(out, "result.txt"))
	if err != nil || string(got) != "ran\n" {
		t.Fatalf("result.txt: %q %v", got, err)
	}
	if !strings.Contains(stderr.String(), "build: tool writer runs trusted") {
		t.Errorf("runArtifact should log the same trusted-mode line as runTool: %s", stderr.String())
	}
}

// TestRunArtifactValidation covers the resolve-time checks: runArtifact
// needs an executable artifact built for the host, not a library or a
// cross-compiled one.
func TestRunArtifactValidation(t *testing.T) {
	t.Parallel()
	host := driver.HostTarget()
	crossArch := "aarch64"
	if host.Arch == "arm64" {
		crossArch = "x86_64"
	}
	g := &buildgraph.Graph{Artifacts: []*buildgraph.Artifact{
		{ID: 0, Name: "lib", Kind: "library", Target: buildgraph.Target{Triple: "x86_64-freestanding"}},
		{ID: 1, Name: "cross", Kind: "executable", Target: buildgraph.Target{Triple: crossArch + "-linux"}},
	}, ToolSteps: []buildgraph.ToolStep{{ID: 0, Kind: "artifact", Artifact: 0, Outputs: []int{}}}}
	if _, err := driver.ResolveGraph(g, nil); err == nil || !strings.Contains(err.Error(), "not an executable") {
		t.Fatalf("runArtifact on a library should be rejected: %v", err)
	}
	g.ToolSteps[0].Artifact = 1
	if _, err := driver.ResolveGraph(g, nil); err == nil || !strings.Contains(err.Error(), "not built for the host") {
		t.Fatalf("runArtifact on a cross-compiled executable should be rejected: %v", err)
	}
	g.ToolSteps[0].Artifact = 2
	if _, err := driver.ResolveGraph(g, nil); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("runArtifact on a nonexistent artifact should be rejected: %v", err)
	}
}

// TestResolveGraphHostBareLibrary covers a build-program path: a
// `library` artifact targeting a hosted OS with
// `--sys none` (Target.Bare(), as `b.request().target()` carries when
// `kigumi build --sys none` drives the whole build.kg) resolves like a
// freestanding one, and an `executable` artifact under the same profile
// is still refused as a runArtifact host tool, since it builds an archive
// rather than something runnable.
func TestResolveGraphHostBareLibrary(t *testing.T) {
	t.Parallel()
	// An empty Triple with Sys "none" is what b.request().target() records
	// when `kigumi build --sys none` (no --target) drives the build.kg.
	g := &buildgraph.Graph{Artifacts: []*buildgraph.Artifact{
		{ID: 0, Name: "enclave", Kind: "library", Target: buildgraph.Target{Sys: "none"}},
	}}
	if _, err := driver.ResolveGraph(g, nil); err != nil {
		t.Fatalf("a host target + --sys none library should resolve: %v", err)
	}
	g.Artifacts[0].Kind = "executable"
	g.ToolSteps = []buildgraph.ToolStep{{ID: 0, Kind: "artifact", Artifact: 0, Outputs: []int{}}}
	if _, err := driver.ResolveGraph(g, nil); err == nil || !strings.Contains(err.Error(), "not built for the host") {
		t.Fatalf("runArtifact on a --sys none archive should be rejected even on the host OS/arch: %v", err)
	}
}
