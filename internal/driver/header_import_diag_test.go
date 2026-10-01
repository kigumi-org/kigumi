package driver_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// TestHeaderImportUnsupportedOnly: a header that
// declares only unsupported constructs (a function-like macro, an inline
// function, an extern variable) imports nothing, but LoadModule still
// surfaces one E994 diagnostic naming every one of them.
func TestHeaderImportUnsupportedOnly(t *testing.T) {
	if _, err := exec.LookPath("zig"); err != nil {
		t.Skip("no zig")
	}
	root, err := filepath.Abs("../../testdata/cheader_unsupported")
	if err != nil {
		t.Fatal(err)
	}
	std, err := filepath.Abs("../../std")
	if err != nil {
		t.Fatal(err)
	}
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std})
	if err != nil {
		t.Fatal(err)
	}
	d := m.Diagnostics()
	if !strings.Contains(d, "E994") {
		t.Fatalf("expected an E994 diagnostic, got:\n%s", d)
	}
	for _, name := range []string{"SQUARE", "inline_add", "global_counter"} {
		if !strings.Contains(d, name) {
			t.Errorf("diagnostic does not name %q:\n%s", name, d)
		}
	}
	if m.HasParseErrors() {
		t.Error("a skip diagnostic must be a warning, not an error")
	}
	// The message must actually come from sem.HeaderImportSkipped's own
	// catalog template (pluralized "declarations", plus its help line) so
	// `kigumi explain E994` describes what a user really sees, rather than
	// header_import.go hand-rolling a differently-worded string.
	if !strings.Contains(d, "skipped 3 declarations:") {
		t.Errorf("diagnostic does not match E994's catalog template:\n%s", d)
	}
	if !strings.Contains(d, "add the missing pieces by hand if the program needs them") {
		t.Errorf("diagnostic is missing E994's catalog help text:\n%s", d)
	}
}

// TestHeaderImportUnsupportedOnlyUnreachable covers the same requirement as
// TestHeaderImportUnsupportedOnly but through the CLI's real loading path,
// where the entry's own import graph never reaches the header's generated
// c/onlyunsupported package.
func TestHeaderImportUnsupportedOnlyUnreachable(t *testing.T) {
	if _, err := exec.LookPath("zig"); err != nil {
		t.Skip("no zig")
	}
	root, err := filepath.Abs("../../testdata/cheader_unsupported")
	if err != nil {
		t.Fatal(err)
	}
	std, err := filepath.Abs("../../std")
	if err != nil {
		t.Fatal(err)
	}
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Entry: "main.kg"})
	if err != nil {
		t.Fatal(err)
	}
	if !m.ExplicitEntry {
		t.Fatal("Entry set should make ExplicitEntry true")
	}
	d := m.Diagnostics()
	if !strings.Contains(d, "E994") {
		t.Fatalf("an explicit entry that never imports the header package must still surface E994, got:\n%s", d)
	}
	if m.HasParseErrors() {
		t.Error("a skip diagnostic must be a warning, not an error")
	}
}

// TestBuildDiagnosticsNotDuplicated covers the `kigumi build` path: Load
// (cli/shared) prints a module's diagnostics once, and build.go's compile
// must not print the same *Module's diagnostics again.
func TestBuildDiagnosticsNotDuplicated(t *testing.T) {
	if _, err := exec.LookPath("zig"); err != nil {
		t.Skip("no zig")
	}
	root, err := filepath.Abs("../../testdata/cheader_unsupported")
	if err != nil {
		t.Fatal(err)
	}
	std, err := filepath.Abs("../../std")
	if err != nil {
		t.Fatal(err)
	}
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: std, Entry: "main.kg"})
	if err != nil {
		t.Fatal(err)
	}
	var loadOut strings.Builder
	if m.ReportDiagnostics(&loadOut) {
		t.Fatalf("expected a warning, not a parse error:\n%s", loadOut.String())
	}
	if !strings.Contains(loadOut.String(), "E994") {
		t.Fatalf("Load's own print should surface E994, got:\n%s", loadOut.String())
	}
	var compileOut strings.Builder
	if _, _, err := driver.Compile(m, &compileOut); err != nil {
		t.Fatal(err)
	}
	if compileOut.String() != "" {
		t.Errorf("compile must not reprint diagnostics Load already printed, got:\n%s", compileOut.String())
	}
}
