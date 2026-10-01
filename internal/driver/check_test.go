package driver_test

import (
	"bytes"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// TestRepostatChecks: the example type-checks against the std stubs with
// no diagnostics at all.
func TestRepostatChecks(t *testing.T) {
	t.Parallel()
	m, err := driver.LoadModule("../../examples/repostat", driver.LoadOptions{Test: true, StdRoot: "../../std"})
	if err != nil {
		t.Fatal(err)
	}
	if d := m.Diagnostics(); d != "" {
		t.Fatalf("parse diagnostics:\n%s", d)
	}
	res, err := driver.Check(m)
	if err != nil {
		t.Fatal(err)
	}
	if out := res.Render(); out != "" {
		t.Fatalf("diagnostics:\n%s", out)
	}
}

// TestRepostatTests runs the example's `test` blocks in the interpreter.
func TestRepostatTests(t *testing.T) {
	t.Parallel()
	m, err := driver.LoadModule("../../examples/repostat", driver.LoadOptions{Test: true, StdRoot: "../../std"})
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code, err := driver.RunTests(m, &out, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("tests failed (%d): %v\n%s%s", code, err, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "ok   ") {
		t.Fatalf("no tests ran:\n%s", out.String())
	}
}
