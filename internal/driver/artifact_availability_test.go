package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// artifactAvailabilityModule writes a module whose build.kg declares one
// executable artifact ("art") and calls Artifact.availability(deny,
// ceiling) on it; entryBody becomes cmd/art/main.kg.
func artifactAvailabilityModule(t *testing.T, entryBody, deny, ceiling string) (root, out string) {
	t.Helper()
	root, out = t.TempDir(), t.TempDir()
	write := func(rel, body string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755)
		os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644)
	}
	write("mod.kg", "Module {\n    name: \"av\"\n    kigumi: \"0.1\"\n}\n")
	write("cmd/art/main.kg", entryBody)
	program := "import build from std/build\n\nfn configure(b: build.Builder) -> Unit! {\n" +
		"    let a = b.executable(\"art\", \"cmd/art\", b.request().target())?\n" +
		"    a.availability(" + deny + ", " + ceiling + ")\n" +
		"}\n"
	write("build.kg", program)
	return root, out
}

// runArtifactAvailability loads and executes root's build program, and
// returns whatever ExecuteGraph and its stderr reported.
func runArtifactAvailability(t *testing.T, root, out string) (stderr string, err error) {
	t.Helper()
	std, _ := filepath.Abs("../../std")
	p, _, loadErr := driver.LoadBuildProgram(root, driver.LoadOptions{StdRoot: std})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	g, runErr := p.Run(driver.HostTarget(), nil, out)
	if runErr != nil {
		t.Fatal(runErr)
	}
	rg, resolveErr := driver.ResolveGraph(g, nil)
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	var buf bytes.Buffer
	err = driver.ExecuteGraph(p, rg, out, driver.ExecOptions{}, &buf)
	return buf.String(), err
}

// TestArtifactAvailabilityDeniesImport covers Artifact.availability's deny
// list: an
// artifact that bars "std/time" fails to check a package importing it,
// with E969 named at the import.
func TestArtifactAvailabilityDeniesImport(t *testing.T) {
	t.Parallel()
	root, out := artifactAvailabilityModule(t,
		"import time from std/time\n\nprint \"hi\"\n",
		`Array.of("std/time")`, `""`)
	stderr, err := runArtifactAvailability(t, root, out)
	if err == nil {
		t.Fatalf("expected the build to fail; stderr:\n%s", stderr)
	}
	for _, want := range []string{"E969", "std/time", "is not available in artifact", "art"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
}

// TestArtifactAvailabilityAllowsUndenied covers the other side: a package
// that never imports a denied path still builds.
func TestArtifactAvailabilityAllowsUndenied(t *testing.T) {
	t.Parallel()
	if driver.CCompiler() == nil {
		t.Skip("no C compiler")
	}
	root, out := artifactAvailabilityModule(t, "print \"hi\"\n", `Array.of("std/time")`, `""`)
	stderr, err := runArtifactAvailability(t, root, out)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, stderr)
	}
	if _, statErr := os.Stat(filepath.Join(out, "art")); statErr != nil {
		t.Errorf("artifact was not built: %v", statErr)
	}
}

// TestArtifactAvailabilityDenyRejectsUnknownPath covers a typo'd deny
// entry: a path that names no std package must abort configure rather
// than silently deny nothing, since a silently-ignored typo would defeat
// the SGX trusted-only guarantee this feature exists for.
func TestArtifactAvailabilityDenyRejectsUnknownPath(t *testing.T) {
	t.Parallel()
	root, out := artifactAvailabilityModule(t,
		"import time from std/time\n\nprint \"hi\"\n",
		`Array.of("std/tiem")`, `""`)
	std, _ := filepath.Abs("../../std")
	p, _, loadErr := driver.LoadBuildProgram(root, driver.LoadOptions{StdRoot: std})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	_, runErr := p.Run(driver.HostTarget(), nil, out)
	if runErr == nil {
		t.Fatal("expected configure to abort on an unknown deny path")
	}
	if !strings.Contains(runErr.Error(), "std/tiem") {
		t.Errorf("error lacks the offending path: %v", runErr)
	}
}

// TestArtifactAvailabilityCeiling covers the layer-ceiling form: a "core"
// ceiling bars a Platform-layer package (std/time) transitively, without
// naming it in deny.
func TestArtifactAvailabilityCeiling(t *testing.T) {
	t.Parallel()
	root, out := artifactAvailabilityModule(t,
		"import time from std/time\n\nprint \"hi\"\n",
		`Array.empty[String]()`, `"core"`)
	stderr, err := runArtifactAvailability(t, root, out)
	if err == nil {
		t.Fatalf("expected the build to fail; stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "E969") || !strings.Contains(stderr, "std/time") {
		t.Errorf("stderr lacks the expected diagnostic:\n%s", stderr)
	}
}
