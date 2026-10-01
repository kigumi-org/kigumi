package mir_test

import (
	"strings"
	"testing"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/testkit"
	"kigumi/internal/token"
)

// program checks a tiny script so the hand-built functions below have a
// type table and a Result to verify against.
func program(t *testing.T) *mir.Program {
	t.Helper()
	res := checkSourceMir(t, "print \"x\"\n")
	return &mir.Program{R: res, ByEnt: map[sem.EntityID]*mir.Func{}}
}

func checkSourceMir(t *testing.T, src string) *sem.Result {
	t.Helper()
	main := &sem.Package{Path: "main"}
	main.Entry = syntax.Parse(token.NewFile("main/main.kg", []byte(src)))
	main.Files = []*syntax.Tree{main.Entry}
	res := sem.Check(&sem.Module{Packages: append(testkit.LoadStd(t, "../../std"), main)})
	if res.HasErrors() {
		t.Fatalf("check errors:\n%s", res.Render())
	}
	return res
}

// resource is a move-only type: the Error interface boxed.
func resource(p *mir.Program) sem.TypeID {
	return p.R.Types.Iface(p.R.Types.ErrorEnt(), nil)
}

func ret(args ...mir.LocalID) mir.Term { return mir.Term{Op: mir.TermReturn, Args: args} }

func fn(name string, ret sem.TypeID, locals []mir.Local, blocks ...mir.Block) *mir.Func {
	return &mir.Func{Name: name, Ret: ret, Locals: locals, Blocks: blocks}
}

// expectRule verifies one function and wants the named rule to fire.
func expectRule(t *testing.T, p *mir.Program, f *mir.Func, rule, fragment string) {
	t.Helper()
	p.Funcs = []*mir.Func{f}
	err := p.Verify()
	if err == nil {
		t.Fatalf("%s: verified, want a %s error", f.Name, rule)
	}
	ve, ok := err.(*mir.Error)
	if !ok || ve.Rule != rule || !strings.Contains(ve.Msg, fragment) {
		t.Fatalf("%s: got %v, want rule %s mentioning %q", f.Name, err, rule, fragment)
	}
}

func TestVerifyStructure(t *testing.T) {
	p := program(t)
	unit := []mir.Local{{Name: "t0", Type: sem.TyUnit}}
	expectRule(t, p, fn("noterm", sem.TyUnit, unit, mir.Block{Insts: []mir.Inst{{Op: mir.OpUnit, Dst: 0}}}), "structure", "missing terminator")
	expectRule(t, p, fn("range", sem.TyUnit, unit, mir.Block{Insts: []mir.Inst{{Op: mir.OpCopy, Dst: 0, Args: []mir.LocalID{7}}}, Term: ret(0)}), "structure", "out of range")
	expectRule(t, p, fn("target", sem.TyUnit, unit, mir.Block{Insts: []mir.Inst{{Op: mir.OpUnit, Dst: 0}}, Term: mir.Term{Op: mir.TermJump, Targets: []mir.BlockID{3}}}), "structure", "target b3 out of range")
	expectRule(t, p, fn("branch", sem.TyUnit, unit, mir.Block{Insts: []mir.Inst{{Op: mir.OpUnit, Dst: 0}}, Term: mir.Term{Op: mir.TermBranch, Args: []mir.LocalID{0}, Targets: []mir.BlockID{0}}}), "structure", "two targets")
}

func TestVerifyShape(t *testing.T) {
	p := program(t)
	locals := []mir.Local{{Name: "t0", Type: sem.TyI64}, {Name: "t1", Type: sem.TyI64}}
	expectRule(t, p, fn("arity", sem.TyI64, locals, mir.Block{Insts: []mir.Inst{{Op: mir.OpBinary, Dst: 0, Str: "+", Args: []mir.LocalID{1}}}, Term: ret(0)}), "shape", "takes 2 arguments")
	expectRule(t, p, fn("notype", sem.TyI64, locals, mir.Block{Insts: []mir.Inst{{Op: mir.OpConst, Dst: 0}}, Term: ret(0)}), "shape", "needs a type")
	expectRule(t, p, fn("noent", sem.TyI64, locals, mir.Block{Insts: []mir.Inst{{Op: mir.OpFnItem, Dst: 0}}, Term: ret(0)}), "shape", "needs an entity")
	expectRule(t, p, fn("holes", sem.TyString, []mir.Local{{Name: "t0", Type: sem.TyString}, {Name: "t1", Type: sem.TyI64}}, mir.Block{Insts: []mir.Inst{{Op: mir.OpConst, Dst: 1, Type: sem.TyI64, Lit: sem.Literal{Kind: sem.LitInt}}, {Op: mir.OpInterp, Dst: 0, Strs: []string{"a"}, Args: []mir.LocalID{1}}}, Term: ret(0)}), "shape", "holes")
}

func TestVerifyTypes(t *testing.T) {
	p := program(t)
	locals := []mir.Local{{Name: "t0", Type: sem.TyI64}, {Name: "t1", Type: sem.TyBool}, {Name: "t2", Type: sem.TyString}}
	boolConst := mir.Inst{Op: mir.OpConst, Dst: 1, Type: sem.TyBool, Lit: sem.Literal{Kind: sem.LitBool}}
	expectRule(t, p, fn("copy", sem.TyI64, locals, mir.Block{Insts: []mir.Inst{boolConst, {Op: mir.OpCopy, Dst: 0, Args: []mir.LocalID{1}}}, Term: ret(0)}), "types", "copy from Bool into Int")
	expectRule(t, p, fn("const", sem.TyI64, locals, mir.Block{Insts: []mir.Inst{{Op: mir.OpConst, Dst: 0, Type: sem.TyBool, Lit: sem.Literal{Kind: sem.LitBool}}}, Term: ret(0)}), "types", "constant of Bool into Int")
	expectRule(t, p, fn("cond", sem.TyI64, locals, mir.Block{Insts: []mir.Inst{{Op: mir.OpConst, Dst: 0, Type: sem.TyI64, Lit: sem.Literal{Kind: sem.LitInt}}}, Term: mir.Term{Op: mir.TermBranch, Args: []mir.LocalID{0}, Targets: []mir.BlockID{1, 1}}}, mir.Block{Term: ret(0)}), "types", "branch on Int")
	expectRule(t, p, fn("ret", sem.TyI64, locals, mir.Block{Insts: []mir.Inst{boolConst}, Term: ret(1)}), "types", "return of Bool")
	expectRule(t, p, fn("interp", sem.TyI64, locals, mir.Block{Insts: []mir.Inst{{Op: mir.OpInterp, Dst: 0, Strs: []string{"x"}}}, Term: ret(0)}), "types", "yields String")
	// A bare Unit only stands for Ok(()) on the ImplicitOk script exit
	// (build.go); an ordinary fallible-Unit function must return an Ok.
	unitRet := p.R.Types.Result(sem.TyUnit, resource(p))
	unitLocals := []mir.Local{{Name: "t0", Type: sem.TyUnit}}
	expectRule(t, p, fn("bareunit", unitRet, unitLocals, mir.Block{Insts: []mir.Inst{{Op: mir.OpUnit, Dst: 0}}, Term: ret(0)}), "types", "return of Unit")
	implicitOk := fn("main", unitRet, unitLocals, mir.Block{Insts: []mir.Inst{{Op: mir.OpUnit, Dst: 0}}, Term: ret(0)})
	implicitOk.ImplicitOk = true
	p.Funcs = []*mir.Func{implicitOk}
	if err := p.Verify(); err != nil {
		t.Fatalf("ImplicitOk bare Unit return: got %v, want no error", err)
	}
}

// TestVerifyFnPurityErasure checks that a copy from a plain fn value into a
// `pure`/`noalloc`-qualified fn-typed slot (LAM-2, or a plain fn item)
// verifies, since purity is the effects pass's job, not the runtime
// representation's.
func TestVerifyFnPurityErasure(t *testing.T) {
	p := program(t)
	tt := p.R.Types
	plain := tt.Fn([]sem.TypeID{sem.TyI64}, sem.TyI64, 0, false)
	pure := tt.Fn([]sem.TypeID{sem.TyI64}, sem.TyI64, sem.EffPure, false)
	locals := []mir.Local{{Name: "t0", Type: pure}, {Name: "t1", Type: plain}}
	f := fn("erase", pure, locals, mir.Block{Insts: []mir.Inst{{Op: mir.OpCopy, Dst: 0, Args: []mir.LocalID{1}}}, Term: ret(0)})
	f.Params = []mir.LocalID{1}
	p.Funcs = []*mir.Func{f}
	if err := p.Verify(); err != nil {
		t.Fatalf("erase: got %v, want no error", err)
	}
}

func TestVerifyDefs(t *testing.T) {
	p := program(t)
	locals := []mir.Local{{Name: "t0", Type: sem.TyBool}, {Name: "t1", Type: sem.TyI64}, {Name: "t2", Type: sem.TyI64}}
	// t1 is assigned on one branch only, then read at the join.
	f := fn("halfdef", sem.TyI64, locals,
		mir.Block{Insts: []mir.Inst{{Op: mir.OpConst, Dst: 0, Type: sem.TyBool, Lit: sem.Literal{Kind: sem.LitBool}}}, Term: mir.Term{Op: mir.TermBranch, Args: []mir.LocalID{0}, Targets: []mir.BlockID{1, 2}}},
		mir.Block{Insts: []mir.Inst{{Op: mir.OpConst, Dst: 1, Type: sem.TyI64, Lit: sem.Literal{Kind: sem.LitInt}}}, Term: mir.Term{Op: mir.TermJump, Targets: []mir.BlockID{2}}},
		mir.Block{Insts: []mir.Inst{{Op: mir.OpCopy, Dst: 2, Args: []mir.LocalID{1}}}, Term: ret(2)})
	expectRule(t, p, f, "defs", "reads %t1 before any definition")
}

func TestVerifyOwnership(t *testing.T) {
	p := program(t)
	res := resource(p)
	ent := p.R.Types.ErrorEnt()
	locals := []mir.Local{{Name: "r", Type: res, Ent: ent}, {Name: "t1", Type: res}, {Name: "t2", Type: sem.TyUnit}}
	unit := mir.Inst{Op: mir.OpUnit, Dst: 2}
	leak := fn("leak", sem.TyUnit, locals, mir.Block{Insts: []mir.Inst{unit}, Term: ret(2)})
	leak.Params = []mir.LocalID{0}
	expectRule(t, p, leak, "ownership", "still owned at return")
	twice := fn("twice", sem.TyUnit, locals, mir.Block{Insts: []mir.Inst{{Op: mir.OpDrop, Args: []mir.LocalID{0}}, {Op: mir.OpDrop, Args: []mir.LocalID{0}}, unit}, Term: ret(2)})
	twice.Params = []mir.LocalID{0}
	expectRule(t, p, twice, "ownership", "holds nothing")
	after := fn("after", sem.TyUnit, locals, mir.Block{Insts: []mir.Inst{{Op: mir.OpMove, Dst: 1, Args: []mir.LocalID{0}}, {Op: mir.OpCopy, Dst: 1, Args: []mir.LocalID{0}}, unit}, Term: ret(2)})
	after.Params = []mir.LocalID{0}
	expectRule(t, p, after, "ownership", "after it was consumed")
	over := fn("over", sem.TyUnit, locals, mir.Block{Insts: []mir.Inst{{Op: mir.OpMove, Dst: 1, Args: []mir.LocalID{0}}, {Op: mir.OpMove, Dst: 0, Args: []mir.LocalID{1}}, {Op: mir.OpMove, Dst: 0, Args: []mir.LocalID{1}}, unit}, Term: ret(2)})
	over.Params = []mir.LocalID{0}
	expectRule(t, p, over, "ownership", "still holds a value")
}

// TestVerifyCellCaptures checks that a closure's captured `mut self` is
// backed by an OpNewCell, and that breaking a real self capture's
// OpNewCell is caught as a cellcapture error.
func TestVerifyCellCaptures(t *testing.T) {
	res := checkSourceMir(t, `
type Counter = {
    pub mut n Int
}
fn Counter.bump(mut self) -> Unit {
    let step = () => { self.n = self.n + 1 }
    step()
}
let mut c = Counter { n: 0 }
c.bump()
`)
	prog := mir.Build(res)
	if err := prog.Verify(); err != nil {
		t.Fatalf("verify: %v", err)
	}
	var target *mir.Func
	for _, f := range prog.Funcs {
		if strings.Contains(f.Name, "Counter.bump") {
			target = f
		}
	}
	if target == nil {
		t.Fatal("Counter.bump not found among the built functions")
	}
	patched := false
	for bi := range target.Blocks {
		for ii := range target.Blocks[bi].Insts {
			if target.Blocks[bi].Insts[ii].Op == mir.OpNewCell {
				target.Blocks[bi].Insts[ii].Op = mir.OpCopy
				patched = true
			}
		}
	}
	if !patched {
		t.Fatal("Counter.bump has no OpNewCell to break")
	}
	err := prog.Verify()
	ve, ok := err.(*mir.Error)
	if !ok || ve.Rule != "cellcapture" {
		t.Fatalf("got %v, want a cellcapture error", err)
	}
}

// TestVerifyCellCapturesNested checks that a closure forwarding its own
// captured cell to a nested closure verifies: build.go never rewraps a
// forwarded capture, and the trace back to the outer OpNewCell must follow
// it through both layers.
func TestVerifyCellCapturesNested(t *testing.T) {
	res := checkSourceMir(t, `
type Counter = {
    pub mut n Int
}
fn Counter.nested(mut self) -> Unit {
    let outer = () => {
        let inner = () => { self.n = self.n + 1 }
        inner()
    }
    outer()
}
let mut c = Counter { n: 0 }
c.nested()
`)
	prog := mir.Build(res)
	if err := prog.Verify(); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := prog.Apply(mir.Transform{Name: "plan-rc", Run: mir.PlanRC}); err != nil {
		t.Fatalf("plan-rc: %v\n%s", err, prog.Dump())
	}
}
