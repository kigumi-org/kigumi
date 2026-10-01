package sem_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/diag"
	"kigumi/internal/sem"
	"kigumi/internal/testkit"
)

// TestSemCorpus runs every testdata/sem fixture against its annotations and
// its -- expect -- section; UPDATE=1 rewrites the expect section.
func TestSemCorpus(t *testing.T) {
	files, err := filepath.Glob("../../testdata/sem/*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no fixtures under testdata/sem")
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			fx := parseFixture(t, path)
			if p := fx.opt("pending", ""); p != "" {
				t.Skipf("pending: %s", p)
			}
			mod := buildModule(t, fx)
			res := sem.Check(mod)
			bodies := map[string]string{}
			for _, s := range fx.sections {
				name := s.name
				if name == "src" {
					name = "main/main.kg"
				}
				bodies[name] = s.body
			}
			var got strings.Builder
			for i := 1; i < len(res.Files); i++ {
				tree := res.Files[i].Tree
				got.WriteString(diag.RenderAll(res, res.Diagnostics(tree)))
				if body, ok := bodies[tree.File.Name]; ok {
					checkAnnotations(t, res, tree, body)
				} else if len(res.Diagnostics(tree)) > 0 {
					t.Errorf("shared stub %s reported diagnostics", tree.File.Name)
				}
			}
			if os.Getenv("UPDATE") == "1" {
				out := fx.raw[:fx.expectOff] + got.String()
				if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			if got.String() != fx.expect {
				t.Errorf("expect mismatch (UPDATE=1 to accept)\n--- got ---\n%s--- want ---\n%s", got.String(), fx.expect)
			}
		})
	}
}

// TestStdStubs checks the shared stubs alone: they must be clean.
func TestStdStubs(t *testing.T) {
	mod := &sem.Module{Packages: testkit.LoadStd(t, "../../std")}
	res := sem.Check(mod)
	if out := res.Render(); out != "" {
		t.Errorf("std stubs are not clean:\n%s", out)
	}
}
