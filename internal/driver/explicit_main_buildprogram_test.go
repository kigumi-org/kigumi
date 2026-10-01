package driver_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kigumi/internal/driver"
)

// TestBuildProgramExplicitMainArtifact covers a build.kg artifact whose
// cmd/ entry is written as an explicit `fn main() -> Unit!` instead of a
// bare-statement script: buildArtifact's scriptUnder must find it, like
// Module.defaultEntry does.
func TestBuildProgramExplicitMainArtifact(t *testing.T) {
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root, out := t.TempDir(), t.TempDir()
	write := func(rel, body string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755)
		os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644)
	}
	write("mod.kg", "Module {\n    name: \"explicitmain\"\n    kigumi: \"0.1\"\n}\n")
	write("cmd/app/main.kg", "fn main() -> Unit! {\n    print(\"explicit main artifact\")\n}\n")
	write("build.kg", "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n"+
		"    let app = b.executable(\"app\", \"cmd/app\", b.request().target())?\n"+
		"}\n")
	std, _ := filepath.Abs("../../std")
	p, ok, err := driver.LoadBuildProgram(root, driver.LoadOptions{StdRoot: std})
	if err != nil || !ok {
		t.Fatalf("load: %v %v", ok, err)
	}
	g, err := p.Run(driver.HostTarget(), nil, out)
	if err != nil {
		t.Fatal(err)
	}
	rg, err := driver.ResolveGraph(g, nil)
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if err := driver.ExecuteGraph(p, rg, out, driver.ExecOptions{}, &stderr); err != nil {
		t.Fatalf("execute: %v\n%s", err, stderr.String())
	}
	got, err := exec.Command(filepath.Join(out, "app")).Output()
	if err != nil || string(got) != "explicit main artifact\n" {
		t.Fatalf("app: %q %v", got, err)
	}
}
