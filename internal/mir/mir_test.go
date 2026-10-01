package mir_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/testkit"
	"kigumi/internal/token"
)

// Each testdata/mir fixture holds source sections and an `-- expect --`
// section with the IR dump followed by ownership diagnostics.
func TestMirCorpus(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/mir/*.txtar")
	if len(files) == 0 {
		t.Fatal("no mir fixtures")
	}
	std := testkit.LoadStd(t, "../../std")
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			runFixture(t, path, std)
		})
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

// checkFixture loads a fixture's packages over std and type-checks them;
// the raw text and the reassembled source sections come back for goldens.
func checkFixture(t *testing.T, path string, std []*sem.Package) (*sem.Result, string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pkgs := map[string]*sem.Package{}
	for _, p := range std {
		pkgs[p.Path] = p
	}
	var head string
	for _, s := range parseTxtar(string(data)) {
		switch s.name {
		case "expect", "stdout", "stderr", "exit", "engines":
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
		head += "-- " + s.name + " --\n" + s.body
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
	return res, string(data), head
}

func runFixture(t *testing.T, path string, std []*sem.Package) {
	res, data, head := checkFixture(t, path, std)
	prog := mir.Build(res)
	// The contract holds for programs the builder accepted; a fixture that
	// expects ownership diagnostics is checked on its rendering alone.
	if !prog.HasErrors() {
		if err := prog.Verify(); err != nil {
			t.Fatalf("verify: %v\n%s", err, prog.Dump())
		}
	}
	got := userDump(prog) + prog.Render()
	want := ""
	if i := strings.Index(data, "-- expect --\n"); i >= 0 {
		want = data[i+len("-- expect --\n"):]
	}
	if os.Getenv("UPDATE") == "1" {
		os.WriteFile(path, []byte(head+"-- expect --\n"+got), 0o644)
		return
	}
	if got != want {
		t.Errorf("expect mismatch (UPDATE=1 to accept)\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

// userDump leaves the std bodies, and the closures inside them, out of the
// golden output.
func userDump(p *mir.Program) string {
	user := &mir.Program{R: p.R, ByEnt: p.ByEnt}
	for _, f := range p.Funcs {
		root := f.Ent
		for p.R.Entity(root).Kind == sem.EntClosure {
			root = p.R.Entity(root).Parent
		}
		if p.R.Entity(root).Flags&sem.EfStd == 0 {
			user.Funcs = append(user.Funcs, f)
		}
	}
	return user.Dump()
}
