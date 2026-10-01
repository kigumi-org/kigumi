package driver

import (
	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

// Analysis is the checked view of a module, published complete and never written afterward.
type Analysis struct {
	Res *sem.Result
	// Prog is nil when the checker reported errors.
	Prog *mir.Program
}

// Stats counts the phase runs of one module, so a test can pin that one
// build invocation checks, lowers and emits exactly once.
type Stats struct {
	Check, MIR, Emit int
}

// Analyze checks a loaded module and lowers it to MIR once. The error is
// a compiler fault: the MIR the builder or a pass produced broke the MIR
// contract (mir.Program.Verify), never a diagnostic of the program.
func Analyze(m *Module) (*Analysis, error) {
	mod, err := m.semModule()
	if err != nil {
		return nil, err
	}
	return m.analyze(mod)
}

func (m *Module) analyze(mod *sem.Module) (*Analysis, error) {
	m.stats.Check++
	res := sem.Check(mod)
	a := &Analysis{Res: res}
	if res.HasErrors() {
		return a, nil
	}
	m.stats.MIR++
	var err error
	if a.Prog, err = buildProgram(res); err != nil {
		return nil, err
	}
	// Several cmd/ scripts each have an implicit main; the module's entry
	// names the one a program starts from.
	if m.Entry != "" {
		for _, ent := range res.ImplicitMains() {
			if res.Tree(res.Entity(ent).File).File.Name == m.Entry {
				a.Prog.Entry = a.Prog.ByEnt[ent]
			}
		}
	}
	return a, nil
}

// buildProgram returns a program with diagnostics unverified: the MIR
// contract holds only for programs the builder accepted, and nothing runs
// an unverified one.
func buildProgram(res *sem.Result) (*mir.Program, error) {
	prog := mir.Build(res)
	for _, f := range prog.Funcs {
		res.Files[f.File].Diags = append(res.Files[f.File].Diags, f.Diags...)
	}
	for _, cf := range prog.Comptime {
		res.Files[cf.Func.File].Diags = append(res.Files[cf.Func.File].Diags, cf.Func.Diags...)
	}
	if res.HasErrors() {
		return prog, nil
	}
	// plan-rc runs first: fold-comptime evaluates comptime blocks in the
	// VM, which only translates the ops plan-rc produces.
	return prog, prog.Apply(
		mir.Transform{Name: "plan-rc", Run: mir.PlanRC},
		mir.Transform{Name: "fold-comptime", Run: foldComptime},
	)
}

// Stats reports how often each phase ran for this module.
func (m *Module) Stats() Stats { return m.stats }
