package vm

import (
	"fmt"
	"io"
	"kigumi/internal/diag"

	"kigumi/internal/mir"
)

// RunTests runs every test block in entity order and reports each as
// `ok` or `FAIL` on out, with the error or panic on errOut; it returns the
// number of failed tests.
func RunTests(prog *mir.Program, opts Options, out, errOut io.Writer) (failed int, err error) {
	m := newMachine(prog, opts, out, errOut)
	for _, f := range testFuncs(prog) {
		name := m.r.Entity(f.Ent).Name
		ok := false
		cerr := m.catch(func() { ok = m.runTest(f, name) })
		if p, isPanic := cerr.(*Panic); isPanic {
			fmt.Fprintf(errOut, "%s: %s\n", diag.Prefix("panic"), p.Msg)
		} else if cerr != nil {
			return failed, cerr
		}
		if ok {
			fmt.Fprintf(out, "ok   %s\n", name)
		} else {
			failed++
			fmt.Fprintf(out, "FAIL %s\n", name)
		}
	}
	return failed, nil
}

func (m *Machine) runTest(f *mir.Func, name string) bool {
	res := deref(m.call(f, nil))
	if res != nil && res.k == kVariant && res.ent == m.variantOf(m.r.Types.ResultEnt(), "Err") {
		fmt.Fprintf(m.err, "%s: %s\n", name, m.display(m.errorMessage(res.fields[0])))
		m.release(res)
		return false
	}
	m.release(res)
	return true
}
