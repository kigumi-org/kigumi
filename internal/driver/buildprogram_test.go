package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// TestBuildProgramExample runs examples/build_program end to end: the
// library archive, the tool step that generates a C source from it, and
// the host executable that links the result.
func TestBuildProgramExample(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	std, _ := filepath.Abs("../../std")
	p, ok, err := driver.LoadBuildProgram("../../examples/build_program", driver.LoadOptions{StdRoot: std})
	if err != nil || !ok {
		t.Fatalf("load: %v %v", ok, err)
	}
	out := t.TempDir()
	g, err := p.Run(driver.HostTarget(), []string{"--big"}, out)
	if err != nil {
		t.Fatal(err)
	}
	roots, _ := p.DescriptorRoots()
	descriptors, err := driver.LoadFrameworks(roots)
	if err != nil {
		t.Fatal(err)
	}
	rg, err := driver.ResolveGraph(g, descriptors)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(rg.Order, " ") != "a0 t0 a1" {
		t.Fatalf("order: %v", rg.Order)
	}
	plan := driver.PlanJSON(rg)
	for _, want := range []string{"\"name\": \"core\"", "\"tool\": \"gen\"", "\"kind\": \"executable\"", "\"order\""} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan lacks %s:\n%s", want, plan)
		}
	}
	var stderr bytes.Buffer
	if err := driver.ExecuteGraph(p, rg, out, driver.ExecOptions{}, &stderr); err != nil {
		t.Fatalf("execute: %v\n%s", err, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(out, "core.a")); err != nil {
		t.Fatal("library archive missing")
	}
	got, err := exec.Command(filepath.Join(out, "app")).Output()
	if err != nil || string(got) != "generated 700\n" {
		t.Fatalf("app: %q %v", got, err)
	}

	g.ToolSteps[0].Tool = "rm"
	if _, err := driver.ResolveGraph(g, descriptors); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("unregistered tool should be rejected before running: %v", err)
	}
	g.ToolSteps[0].Tool = "gen"
	g.Artifacts[0].Target.Triple = ""
	if _, err := driver.ResolveGraph(g, descriptors); err == nil || !strings.Contains(err.Error(), "freestanding") {
		t.Fatalf("host-target library should be rejected: %v", err)
	}
}

// TestBuildProgramShape rejects a build.kg without the fixed entry.
func TestBuildProgramShape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "mod.kg"), []byte("Module {\n    name: \"shape\"\n    kigumi: \"0.1\"\n}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "build.kg"), []byte("import build from std/build\n\nfn configure(b: build.Builder, extra: Int) -> Unit! {\n    let a = b.executable(\"x\", \"\", b.request().target())?\n}\n"), 0o644)
	std, _ := filepath.Abs("../../std")
	if _, _, err := driver.LoadBuildProgram(root, driver.LoadOptions{StdRoot: std}); err == nil || !strings.Contains(err.Error(), "configure must be") {
		t.Fatalf("expected the shape error, got %v", err)
	}
	os.WriteFile(filepath.Join(root, "build.kg"), []byte("import build from std/build\n\nlet x = 1\n\nfn configure(b: build.Builder) -> Unit! {\n    let a = b.executable(\"x\", \"\", b.request().target())?\n}\n"), 0o644)
	if _, _, err := driver.LoadBuildProgram(root, driver.LoadOptions{StdRoot: std}); err == nil {
		t.Fatal("top-level statements in build.kg should be rejected")
	}
}
