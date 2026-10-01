package vm_test

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"kigumi/internal/hir"
	"kigumi/internal/mir"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/testkit"
	"kigumi/internal/token"
	"kigumi/internal/vm"
)

type fixture struct {
	src, stdout string
	exit        int
}

func readFixture(t *testing.T, path string) fixture {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	var fx fixture
	sections := strings.Split(text, "\n-- ")
	for i, s := range sections {
		if i == 0 {
			s = strings.TrimPrefix(s, "-- ")
		}
		name, body, _ := strings.Cut(s, " --\n")
		if i < len(sections)-1 {
			body += "\n"
		}
		switch name {
		case "src":
			fx.src = body
		case "stdout":
			fx.stdout = body
		case "exit":
			fx.exit = int(strings.TrimSpace(body)[0] - '0')
		}
	}
	return fx
}

func checkSource(t *testing.T, src string) *sem.Result {
	pkgs := map[string]*sem.Package{}
	for _, p := range testkit.LoadStd(t, "../../std") {
		pkgs[p.Path] = p
	}
	tree := syntax.Parse(token.NewFile("main/main.kg", []byte(src)))
	pkgs["main"] = &sem.Package{Path: "main", Files: []*syntax.Tree{tree}, Entry: tree}
	var paths []string
	for p := range pkgs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	mod := &sem.Module{}
	for _, p := range paths {
		mod.Packages = append(mod.Packages, pkgs[p])
	}
	return sem.Check(mod)
}

// The scalar fixture runs through HIR, MIR and the VM and prints what the
// tree interpreter and the AOT corpus print for the same source; the HIR
// dump pins what the checker decided.
func TestScalarSlice(t *testing.T) {
	fx := readFixture(t, "../../testdata/run/hir_scalar.txtar")
	res := checkSource(t, fx.src)
	if res.HasErrors() {
		t.Fatalf("check errors:\n%s", res.Render())
	}
	var dump strings.Builder
	for id := 1; id < len(res.Entities); id++ {
		e := res.Entity(sem.EntityID(id))
		if e.File == 0 || res.Packages[e.Pkg].Path != "main" || (e.Kind != sem.EntFn && e.Kind != sem.EntImplicitMain) {
			continue
		}
		if f := hir.Lower(res, sem.EntityID(id)); f != nil {
			dump.WriteString(hir.Dump(res, f))
		}
	}
	prog := mir.Build(res)
	if prog.HasErrors() {
		t.Fatal("ownership errors")
	}
	if err := prog.Apply(mir.Transform{Name: "plan-rc", Run: mir.PlanRC}); err != nil {
		t.Fatalf("plan-rc: %v", err)
	}
	var out, errOut bytes.Buffer
	if code, err := vm.Run(prog, vm.Options{}, &out, &errOut); err != nil || code != 0 {
		t.Fatalf("vm: %v (exit %d)\n%s", err, code, errOut.String())
	}
	if out.String() != fx.stdout {
		t.Errorf("stdout mismatch\n--- got ---\n%s--- want ---\n%s", out.String(), fx.stdout)
	}
	golden := filepath.Join("..", "..", "testdata", "hir", "scalar.hir")
	if os.Getenv("UPDATE") != "" {
		os.WriteFile(golden, []byte(dump.String()), 0o644)
	}
	wantDump, err := os.ReadFile(golden)
	if err != nil || string(wantDump) != dump.String() {
		t.Errorf("HIR dump differs from %s (UPDATE=1 to accept)\n%s", golden, dump.String())
	}
}

// TestRunCorpusOnVM runs every run fixture the VM can execute (the MIR
// built from the checked tree, as the AOT backend uses) and compares the
// output; fixtures Supports rejects (C function pointers, foreign memory) are skipped
// and counted, and everything it accepts has to run to the end.
func TestRunCorpusOnVM(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/run/*.txtar")
	skipped := 0
	for _, path := range files {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			fx := readFixture(t, path)
			res := checkSource(t, fx.src)
			if res.HasErrors() {
				t.Skip("check errors")
			}
			prog := mir.Build(res)
			if prog.HasErrors() {
				t.Skip("ownership errors")
			}
			if err := prog.Apply(mir.Transform{Name: "plan-rc", Run: mir.PlanRC}); err != nil {
				t.Fatalf("plan-rc: %v", err)
			}
			if errs := vm.FoldComptime(prog); len(errs) > 0 {
				t.Fatalf("comptime: %v", errs[0].Err)
			}
			if ok, why := vm.Supports(prog); !ok {
				skipped++
				t.Skip(why)
			}
			var out, errOut bytes.Buffer
			code, err := vm.Run(prog, vm.Options{Cwd: t.TempDir(), Args: []string{"main", "one", "two"}}, &out, &errOut)
			if err != nil {
				t.Fatalf("vm: %v", err)
			}
			if out.String() != fx.stdout || code != fx.exit {
				t.Errorf("mismatch (exit %d, want %d)\n--- got ---\n%s--- want ---\n%s--- stderr ---\n%s", code, fx.exit, out.String(), fx.stdout, errOut.String())
			}
		})
	}
	t.Logf("%d fixtures skipped as unsupported", skipped)
}
