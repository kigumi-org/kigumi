package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildProgramCLI prints the plan of a build.kg-driven module and
// refuses the single-artifact flags with it.
func TestBuildProgramCLI(t *testing.T) {
	std, _ := filepath.Abs("../../std")
	example, _ := filepath.Abs("../../examples/build_program")
	code, out, errOut := execute(t, "--std", std, "build", "--plan", example, "--", "--big")
	if code != 0 || !strings.Contains(out, "\"big\"") || !strings.Contains(out, "\"order\"") {
		t.Fatalf("plan: exit %d\n%s%s", code, out, errOut)
	}
	if code, _, _ := execute(t, "--std", std, "build", "-o", "x", example); code != 2 {
		t.Fatalf("-o with a build.kg should be a usage error, exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(example, "app")); err == nil {
		t.Fatal("--plan must not build anything")
	}
}
