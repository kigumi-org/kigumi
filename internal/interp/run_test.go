package interp_test

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"kigumi/internal/interp"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/testkit"
	"kigumi/internal/token"
)

// Each testdata/run fixture holds source sections (`-- src --` is the entry
// file main/main.kg) and the expected `-- stdout --`, plus optional
// `-- stderr --` and `-- exit --` sections.
func TestRunCorpus(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/run/*.txtar")
	if len(files) == 0 {
		t.Fatal("no run fixtures")
	}
	std := testkit.LoadStd(t, "../../std")
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if fixtureEngines(path) == "native" {
				t.Skip("native only")
			}
			runFixture(t, path, std)
		})
	}
}

// fixtureEngines reads a fixture's optional `-- engines --` section (mirrors
// driver_test's own copy): "native" means a real extern(C) declaration that
// only the AOT backend can run, so TestRunCorpus (which panics on any
// foreign call) skips it rather than asserting a partial trace.
func fixtureEngines(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, s := range parseTxtar(string(data)) {
		if s.name == "engines" {
			return strings.TrimSpace(s.body)
		}
	}
	return ""
}

// TestRunCorpusWithoutAccel runs the string fixtures on the Kigumi bodies
// themselves, so an accelerator cannot hide a divergence in the reference.
func TestRunCorpusWithoutAccel(t *testing.T) {
	t.Setenv("KIGUMI_NO_ACCEL", "1")
	std := testkit.LoadStd(t, "../../std")
	for _, name := range []string{"strings_edge.txtar", "stdlib.txtar"} {
		t.Run(name, func(t *testing.T) { runFixture(t, "../../testdata/run/"+name, std) })
	}
}

type section struct{ name, body string }

func parseTxtar(src string) []section {
	var out []section
	var cur *section
	for _, line := range strings.SplitAfter(src, "\n") {
		trimmed := strings.TrimRight(line, "\n")
		if strings.HasPrefix(trimmed, "-- ") && strings.HasSuffix(trimmed, " --") {
			out = append(out, section{name: strings.TrimSuffix(strings.TrimPrefix(trimmed, "-- "), " --")})
			cur = &out[len(out)-1]
			continue
		}
		if cur != nil {
			cur.body += line
		}
	}
	return out
}

func runFixture(t *testing.T, path string, std []*sem.Package) {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pkgs := map[string]*sem.Package{}
	for _, p := range std {
		pkgs[p.Path] = p
	}
	wantOut, wantErr, wantExit := "", "", 0
	for _, s := range parseTxtar(string(data)) {
		switch s.name {
		case "stdout":
			wantOut = s.body
			continue
		case "stderr":
			wantErr = s.body
			continue
		case "exit":
			wantExit, _ = strconv.Atoi(strings.TrimSpace(s.body))
			continue
		case "engines":
			continue
		}
		name := s.name
		if name == "src" {
			name = "main/main.kg"
		}
		tree := syntax.Parse(token.NewFile(name, []byte(s.body)))
		if tree.HasErrors() {
			t.Fatalf("%s: parse errors:\n%s", name, tree.Dump())
		}
		dir := filepath.ToSlash(filepath.Dir(name))
		p := pkgs[dir]
		if p == nil {
			p = &sem.Package{Path: dir}
			pkgs[dir] = p
		}
		p.Files = append(p.Files, tree)
		if name == "main/main.kg" {
			p.Entry = tree
		}
	}
	mod := &sem.Module{}
	var paths []string
	for p := range pkgs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		mod.Packages = append(mod.Packages, pkgs[p])
	}
	res := sem.Check(mod)
	if res.HasErrors() {
		t.Fatalf("check errors:\n%s", res.Render())
	}
	var out, errOut bytes.Buffer
	in := interp.New(res, &out, &errOut, []string{"main", "one", "two"})
	in.NoAccel = os.Getenv("KIGUMI_NO_ACCEL") != ""
	in.SetCwd(t.TempDir())
	code := in.RunMain()
	if out.String() != wantOut {
		t.Errorf("stdout mismatch\n--- got ---\n%s--- want ---\n%s", out.String(), wantOut)
	}
	if wantErr != "" && !strings.Contains(errOut.String(), strings.TrimSpace(wantErr)) {
		t.Errorf("stderr mismatch\n--- got ---\n%s--- want ---\n%s", errOut.String(), wantErr)
	}
	if code != wantExit {
		t.Errorf("exit code %d, want %d\nstderr:\n%s", code, wantExit, errOut.String())
	}
}
