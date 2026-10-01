package mir_test

import (
	"path/filepath"
	"testing"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
	"kigumi/internal/testkit"
)

// TestPlanRCCorpus runs PlanRC over every mir and run fixture: the result
// must still verify, no OpCopy may survive, and (since every fixture has
// at least one temporary) at least one OpRelease must appear somewhere in
// the whole run, or the release-insertion step is dead.
func TestPlanRCCorpus(t *testing.T) {
	std := testkit.LoadStd(t, "../../std")
	var files []string
	for _, glob := range []string{"../../testdata/mir/*.txtar", "../../testdata/run/*.txtar"} {
		fs, _ := filepath.Glob(glob)
		files = append(files, fs...)
	}
	if len(files) == 0 {
		t.Fatal("no fixtures found")
	}
	sawRelease := false
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			res, _, _ := checkFixture(t, path, std)
			prog := mir.Build(res)
			if prog.HasErrors() {
				t.Skip("fixture expects ownership diagnostics")
			}
			if err := prog.Verify(); err != nil {
				t.Fatalf("verify before plan-rc: %v", err)
			}
			if err := prog.Apply(mir.Transform{Name: "plan-rc", Run: mir.PlanRC}); err != nil {
				t.Fatalf("plan-rc: %v\n%s", err, prog.Dump())
			}
			var funcs []*mir.Func
			funcs = append(funcs, prog.Funcs...)
			for _, cf := range prog.Comptime {
				funcs = append(funcs, cf.Func)
			}
			for _, f := range funcs {
				for _, blk := range f.Blocks {
					for _, in := range blk.Insts {
						if in.Op == mir.OpCopy {
							t.Errorf("%s: OpCopy survived plan-rc", f.Name)
						}
						if in.Op == mir.OpRelease {
							sawRelease = true
						}
					}
				}
			}
		})
	}
	if !sawRelease {
		t.Error("plan-rc never inserted an OpRelease across the whole corpus")
	}
}

// rcLocals builds a locals slice where isNamed marks which indices carry a
// nonzero Ent (as if a real declared local), the rest being temporaries.
func rcLocals(t sem.TypeID, ent sem.EntityID, named ...bool) []mir.Local {
	ls := make([]mir.Local, len(named))
	for i, n := range named {
		ls[i] = mir.Local{Name: "t", Type: t}
		if n {
			ls[i].Ent = ent
		}
	}
	return ls
}

// planRC runs the rewrite alone, without the surrounding Verify calls
// Apply makes: these fixtures each isolate one decision and are not
// otherwise complete, ownership-valid functions (TestPlanRCCorpus checks
// the pass against real, fully valid programs instead).
func planRC(t *testing.T, f *mir.Func, p *mir.Program) {
	t.Helper()
	p.Funcs = []*mir.Func{f}
	if err := mir.PlanRC(p); err != nil {
		t.Fatalf("plan-rc: %v", err)
	}
}

// TestPlanRCAlias: copying a temporary (never a named local) is a pure
// alias, no runtime call.
func TestPlanRCAlias(t *testing.T) {
	p := program(t)
	locals := rcLocals(sem.TyI64, 0, false, false)
	f := fn("alias", sem.TyI64, locals,
		mir.Block{Insts: []mir.Inst{
			{Op: mir.OpConst, Dst: 0, Type: sem.TyI64, Lit: sem.Literal{Kind: sem.LitInt}},
			{Op: mir.OpCopy, Dst: 1, Args: []mir.LocalID{0}},
		}, Term: ret(1)})
	planRC(t, f, p)
	if op := f.Blocks[0].Insts[1].Op; op != mir.OpAlias {
		t.Fatalf("got %s, want alias", op)
	}
}

// TestPlanRCShareFlag: an OpCopy built with Share (a match arm's own
// reference) always becomes an eager OpShare.
func TestPlanRCShareFlag(t *testing.T) {
	p := program(t)
	locals := rcLocals(sem.TyI64, 0, false, false)
	f := fn("shareflag", sem.TyI64, locals,
		mir.Block{Insts: []mir.Inst{
			{Op: mir.OpConst, Dst: 0, Type: sem.TyI64, Lit: sem.Literal{Kind: sem.LitInt}},
			{Op: mir.OpCopy, Dst: 1, Args: []mir.LocalID{0}, Share: true},
		}, Term: ret(1)})
	planRC(t, f, p)
	in := f.Blocks[0].Insts[1]
	if in.Op != mir.OpShare || !in.Copy {
		t.Fatalf("got %s copy=%v, want share copy=true", in.Op, in.Copy)
	}
}

// TestPlanRCShareDefault: copying a named, move-only local into another
// named local shares by retaining (not duplicating) it.
func TestPlanRCShareDefault(t *testing.T) {
	p := program(t)
	res := resource(p)
	ent := p.R.Types.ErrorEnt()
	locals := rcLocals(res, ent, true, true)
	f := fn("sharedefault", res, locals,
		mir.Block{Insts: []mir.Inst{{Op: mir.OpCopy, Dst: 1, Args: []mir.LocalID{0}}}, Term: mir.Term{Op: mir.TermReturn, Args: []mir.LocalID{0}}})
	f.Params = []mir.LocalID{0}
	planRC(t, f, p)
	in := f.Blocks[0].Insts[0]
	if in.Op != mir.OpShare || in.Copy {
		t.Fatalf("got %s copy=%v, want share copy=false (retain)", in.Op, in.Copy)
	}
}

// TestPlanRCBorrowNoShare: copying a value produced by a borrow is a pure
// alias even though the borrow's own destination is named.
func TestPlanRCBorrowNoShare(t *testing.T) {
	p := program(t)
	ent := p.R.Types.ErrorEnt()
	ref := p.R.Types.Ref(sem.TyI64, false)
	locals := []mir.Local{{Name: "x", Type: sem.TyI64, Ent: ent}, {Name: "r", Type: ref, Ent: ent}, {Name: "r2", Type: ref, Ent: ent}}
	f := fn("borrownoshare", sem.TyUnit, locals,
		mir.Block{Insts: []mir.Inst{
			{Op: mir.OpBorrow, Dst: 1, Args: []mir.LocalID{0}},
			{Op: mir.OpCopy, Dst: 2, Args: []mir.LocalID{1}},
		}, Term: ret(2)})
	planRC(t, f, p)
	if op := f.Blocks[0].Insts[1].Op; op != mir.OpAlias {
		t.Fatalf("got %s, want alias (a reference is never shared)", op)
	}
}

// TestPlanRCArgShare: a deferred alias (a Copy-typed named local copied
// into a single-use temporary) only takes its own reference where a later
// instruction actually consumes it, as an OpShare inserted right there.
func TestPlanRCArgShare(t *testing.T) {
	p := program(t)
	ent := p.R.Types.ErrorEnt()
	locals := rcLocals(sem.TyI64, ent, true, false, false)
	f := fn("argshare", sem.TyI64, locals,
		mir.Block{Insts: []mir.Inst{
			{Op: mir.OpCopy, Dst: 1, Args: []mir.LocalID{0}},
			{Op: mir.OpTag, Dst: 2, Args: []mir.LocalID{1}},
		}, Term: ret(2)})
	f.Params = []mir.LocalID{0}
	planRC(t, f, p)
	insts := f.Blocks[0].Insts
	if len(insts) != 3 {
		t.Fatalf("got %d instructions, want 3 (alias, share, tag): %s", len(insts), p.Dump())
	}
	if insts[0].Op != mir.OpAlias {
		t.Fatalf("insts[0] = %s, want alias", insts[0].Op)
	}
	if insts[1].Op != mir.OpShare || !insts[1].Copy || insts[1].Args[0] != 1 {
		t.Fatalf("insts[1] = %+v, want an eager copy-share of local 1", insts[1])
	}
	if insts[2].Op != mir.OpTag || insts[2].Args[0] != insts[1].Dst {
		t.Fatalf("insts[2] = %+v, want tag of the shared value", insts[2])
	}
}

// TestPlanRCReturnShare: returning a named local the scope still owns
// takes its own reference; returning a temporary does not.
func TestPlanRCReturnShare(t *testing.T) {
	p := program(t)
	ent := p.R.Types.ErrorEnt()
	locals := rcLocals(sem.TyI64, ent, true)
	f := fn("returnshare", sem.TyI64, locals, mir.Block{Term: ret(0)})
	f.Params = []mir.LocalID{0}
	planRC(t, f, p)
	insts := f.Blocks[0].Insts
	if len(insts) != 1 || insts[0].Op != mir.OpShare {
		t.Fatalf("got %v, want one share before the return", insts)
	}
	if got := f.Blocks[0].Term.Args[0]; got != insts[0].Dst {
		t.Fatalf("return argument %d, want the shared value %d", got, insts[0].Dst)
	}

	locals2 := rcLocals(sem.TyI64, 0, false)
	f2 := fn("returntemp", sem.TyI64, locals2, mir.Block{Insts: []mir.Inst{
		{Op: mir.OpConst, Dst: 0, Type: sem.TyI64, Lit: sem.Literal{Kind: sem.LitInt}},
	}, Term: ret(0)})
	planRC(t, f2, p)
	if insts := f2.Blocks[0].Insts; len(insts) != 1 {
		t.Fatalf("got %d instructions, want no share for a temporary return: %s", len(insts), p.Dump())
	}
}
