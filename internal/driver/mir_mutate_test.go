package driver_test

import (
	"fmt"
	"math/rand"
	"testing"

	"kigumi/internal/driver"
	"kigumi/internal/llgen"
	"kigumi/internal/mir"
)

// mutate applies one random edit to a random function: an operand or
// destination swapped for another local (possibly out of range), an opcode
// changed, an instruction removed, a terminator dropped or retargeted, a
// type or entity cleared. The result is invalid MIR more often than not.
func mutate(p *mir.Program, r *rand.Rand) string {
	f := p.Funcs[r.Intn(len(p.Funcs))]
	blk := &f.Blocks[r.Intn(len(f.Blocks))]
	nl := mir.LocalID(len(f.Locals))
	pick := func() mir.LocalID { return mir.LocalID(r.Intn(int(nl) + 1)) }
	if len(blk.Insts) == 0 || r.Intn(6) == 0 {
		switch r.Intn(3) {
		case 0:
			blk.Term = mir.Term{}
			return f.Name + ": terminator removed"
		case 1:
			if len(blk.Term.Targets) > 0 {
				blk.Term.Targets[r.Intn(len(blk.Term.Targets))] = mir.BlockID(r.Intn(len(f.Blocks) + 1))
				return f.Name + ": terminator retargeted"
			}
		}
		if len(blk.Term.Args) > 0 {
			blk.Term.Args[0] = pick()
			return f.Name + ": terminator operand swapped"
		}
		return f.Name + ": nothing"
	}
	i := r.Intn(len(blk.Insts))
	in := &blk.Insts[i]
	switch r.Intn(7) {
	case 0:
		if len(in.Args) > 0 {
			in.Args[r.Intn(len(in.Args))] = pick()
			return fmt.Sprintf("%s b%d/%d: operand swapped", f.Name, 0, i)
		}
	case 1:
		in.Dst = pick()
		return fmt.Sprintf("%s /%d: destination swapped", f.Name, i)
	case 2:
		in.Op = mir.Opcode(r.Intn(int(mir.OpNop)) + 1)
		return fmt.Sprintf("%s /%d: opcode changed to %s", f.Name, i, in.Op)
	case 3:
		blk.Insts = append(blk.Insts[:i], blk.Insts[i+1:]...)
		return fmt.Sprintf("%s /%d: instruction removed", f.Name, i)
	case 4:
		in.Type = 0
		return fmt.Sprintf("%s /%d: type cleared", f.Name, i)
	case 5:
		in.Ent = 0
		return fmt.Sprintf("%s /%d: entity cleared", f.Name, i)
	}
	in.Args = append(in.Args, pick())
	return fmt.Sprintf("%s /%d: operand added", f.Name, i)
}

func cloneProgram(p *mir.Program) *mir.Program {
	c := &mir.Program{R: p.R, ByEnt: p.ByEnt, Comptime: p.Comptime}
	for _, f := range p.Funcs {
		nf := *f
		nf.Blocks = make([]mir.Block, len(f.Blocks))
		for i, b := range f.Blocks {
			nb := mir.Block{Insts: make([]mir.Inst, len(b.Insts)), Term: b.Term}
			for k, in := range b.Insts {
				in.Args = append([]mir.LocalID{}, in.Args...)
				nb.Insts[k] = in
			}
			nb.Term.Args = append([]mir.LocalID{}, b.Term.Args...)
			nb.Term.Targets = append([]mir.BlockID{}, b.Term.Targets...)
			nf.Blocks[i] = nb
		}
		nf.Locals = append([]mir.Local{}, f.Locals...)
		c.Funcs = append(c.Funcs, &nf)
		if f == p.Entry {
			c.Entry = &nf
		}
	}
	return c
}

func analyzeFixture(t testing.TB, name string) *mir.Program {
	t.Helper()
	root, _, _, _ := loadRunFixture(t, "../../testdata/run/"+name)
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: "../../std", Entry: "main/main.kg"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := driver.Analyze(m)
	if err != nil || a.Res.HasErrors() {
		t.Fatalf("analyze: %v", err)
	}
	return a.Prog
}

// checkMutant verifies a mutated program; when the verifier accepts it,
// the back end has to emit it without crashing.
func checkMutant(t testing.TB, base *mir.Program, seed int64) {
	t.Helper()
	p := cloneProgram(base)
	r := rand.New(rand.NewSource(seed))
	what := mutate(p, r)
	if err := p.Verify(); err != nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("seed %d (%s): verifier accepted MIR the back end crashed on: %v", seed, what, rec)
		}
	}()
	llgen.EmitProgram(p, llgen.EmitOptions{})
}

// TestMutatedMIR runs a fixed sweep of single-edit mutants of two
// programs through the verifier and the back end.
func TestMutatedMIR(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"adt.txtar", "closures.txtar"} {
		base := analyzeFixture(t, name)
		for seed := int64(0); seed < 800; seed++ {
			checkMutant(t, base, seed)
		}
	}
}

func FuzzMutatedMIR(f *testing.F) {
	base := analyzeFixture(f, "adt.txtar")
	for seed := int64(0); seed < 8; seed++ {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, seed int64) {
		checkMutant(t, base, seed)
	})
}
