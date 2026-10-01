package vm_test

import (
	"bytes"
	"kigumi/internal/buildgraph"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/testkit"
	"kigumi/internal/token"
	"kigumi/internal/vm"
)

func evalBuild(t *testing.T, src string, flags []string) (*buildgraph.Graph, error) {
	t.Helper()
	build := &sem.Package{Path: "build", Files: []*syntax.Tree{syntax.Parse(token.NewFile("build.kg", []byte(src)))}}
	res := sem.Check(&sem.Module{Packages: append(testkit.LoadStd(t, "../../std"), build)})
	if res.HasErrors() {
		t.Fatalf("check errors:\n%s", res.Render())
	}
	fn := res.PackageMember(res.PackageByPath("build"), "configure")
	root, out := t.TempDir(), t.TempDir()
	prog := mir.Build(res)
	if err := prog.Apply(mir.Transform{Name: "plan-rc", Run: mir.PlanRC}); err != nil {
		t.Fatalf("plan-rc: %v", err)
	}
	return vm.EvalBuild(prog, fn, buildgraph.Request{Flags: flags}, buildgraph.Roots{Roots: []string{root, out}, Host: buildgraph.Target{Sys: "posix"}})
}

// TestEvalBuild records artifacts, files, a tool step and links from a
// configure body without touching the file system.
func TestEvalBuild(t *testing.T) {
	src := `import build from std/build

fn configure(b: build.Builder) -> Unit! {
    let sdk = b.framework("acme/sdk", "1.0")
    let key = b.secret("KEY")
    let cfg = b.write("cfg.txt", "hello")?
    let lib = b.library("core", "core", b.target("x86_64-freestanding"))?
    let gen = b.runToolOutput(sdk, "gen", Array.of("--in", "$in:0", "--out", "$out:0", "--key", "$secret:0"), Array.of(cfg, lib.output()), "gen.c", Array.of(key))?
    let app = b.executable("app", "cmd/app", b.request().target())?
    app.addSource(gen)
    app.addLink("m", "")
    app.uses(lib)
    if b.request().flag("sim") {
        app.addLink("sim", "")
    }
}
`
	g, err := evalBuild(t, src, []string{"sim"})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Artifacts) != 2 || g.Artifacts[0].Kind != "library" || g.Artifacts[0].Target.Triple != "x86_64-freestanding" || g.Artifacts[1].Name != "app" || g.Artifacts[1].Dir != "cmd/app" {
		t.Fatalf("artifacts: %+v %+v", g.Artifacts[0], g.Artifacts[1])
	}
	app := g.Artifacts[1]
	if len(app.Uses) != 1 || app.Uses[0] != 0 || len(app.ExtraLinks) != 2 || app.ExtraLinks[1].Library != "sim" || len(app.ExtraSources) != 1 {
		t.Fatalf("app edges: %+v", app)
	}
	if len(g.Frameworks) != 1 || len(g.Secrets) != 1 || g.Secrets[0].Name != "KEY" || len(g.ToolSteps) != 1 {
		t.Fatalf("refs: %+v %+v %+v", g.Frameworks, g.Secrets, g.ToolSteps)
	}
	step := g.ToolSteps[0]
	if step.Tool != "gen" || len(step.Inputs) != 2 || len(step.Outputs) != 1 || g.Files[step.Outputs[0]].Path != "gen.c" || g.Files[step.Inputs[1]].Kind != "artifactOutput" || g.Files[step.Inputs[0]].Kind != "generated" {
		t.Fatalf("step: %+v files %+v", step, g.Files)
	}
	if app.Provenance.Package != "build" || step.Provenance.Package != "build" {
		t.Fatalf("provenance: %+v", app.Provenance)
	}
}

// TestEvalBuildRefusals: configure fails on a path outside the sandbox and
// on host effects.
func TestEvalBuildRefusals(t *testing.T) {
	_, err := evalBuild(t, "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n    let f = b.file(\"../escape.c\")?\n    let app = b.executable(\"app\", \"cmd\", b.request().target())?\n    app.addSource(f)\n}\n", nil)
	if err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("expected a sandbox refusal, got %v", err)
	}
	_, err = evalBuild(t, "import build from std/build\nimport time from std/time\n\nfn configure(b: build.Builder) -> Unit! {\n    let n = time.now()\n    let app = b.executable(\"app\", \"cmd\", b.request().target())?\n}\n", nil)
	if err == nil || !strings.Contains(err.Error(), "not available at compile time") {
		t.Fatalf("expected the host-effect refusal, got %v", err)
	}
	_, err = evalBuild(t, "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n    let a = b.executable(\"app\", \"cmd\", b.request().target())?\n    let d = b.executable(\"app\", \"other\", b.request().target())?\n}\n", nil)
	if err == nil || !strings.Contains(err.Error(), "declared twice") {
		t.Fatalf("expected the duplicate-artifact error, got %v", err)
	}
	_ = bytes.MinRead
	_ = filepath.Join
}

// TestEvalBuildOutputNameRefusals covers BUILD-001/BUILD-002: an output
// name of "." or ".." would resolve to the output directory itself or
// its parent once joined, so declaration must reject it up front.
func TestEvalBuildOutputNameRefusals(t *testing.T) {
	for _, name := range []string{".", ".."} {
		_, err := evalBuild(t, "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n    let f = b.write(\""+name+"\", \"x\")?\n}\n", nil)
		if err == nil || !strings.Contains(err.Error(), "bare file name") {
			t.Errorf("write(%q): expected a bare-name refusal, got %v", name, err)
		}
		_, err = evalBuild(t, "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n    let a = b.executable(\""+name+"\", \"cmd\", b.request().target())?\n}\n", nil)
		if err == nil || !strings.Contains(err.Error(), "bare name") {
			t.Errorf("executable(%q): expected a bare-name refusal, got %v", name, err)
		}
		_, err = evalBuild(t, "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n    let fw = b.framework(\"acme/sdk\", \"1.0\")\n    let o = b.runToolOutput(fw, \"gen\", Array.of(\"$out:0\"), Array.empty[build.FilePath](), \""+name+"\", Array.empty[build.Secret]())?\n}\n", nil)
		if err == nil || !strings.Contains(err.Error(), "bare file name") {
			t.Errorf("runToolOutput(%q): expected a bare-name refusal, got %v", name, err)
		}
	}
}
