package interp

import (
	"fmt"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// RunTests runs every `test` block and reports failures; it returns the
// number of failed tests.
func (in *Interp) RunTests() (failed int) {
	for id := 1; id < len(in.r.Entities); id++ {
		e := in.r.Entity(sem.EntityID(id))
		if e.Kind != sem.EntTest {
			continue
		}
		if in.runTest(sem.EntityID(id)) {
			fmt.Fprintf(in.stdout, "ok   %s\n", e.Name)
		} else {
			failed++
			fmt.Fprintf(in.stdout, "FAIL %s\n", e.Name)
		}
	}
	return failed
}

func (in *Interp) runTest(id sem.EntityID) (ok bool) {
	defer func() {
		if rec := recover(); rec != nil {
			if p, isPanic := rec.(*Panic); isPanic {
				fmt.Fprintln(in.stderr, p.Error())
				ok = false
				return
			}
			panic(rec)
		}
	}()
	e := in.r.Entity(id)
	fr := in.newFrame(id, e.File)
	body := fr.t.Nodes[e.Node].Rhs
	v, c := in.runBody(fr, syntax.NodeID(body), in.r.Types.Result(sem.TyUnit, in.r.Types.Iface(in.r.Types.ErrorEnt(), nil)))
	if c != nil {
		return false
	}
	if vr, isVariant := deref(v).(*Variant); isVariant && in.r.Entity(vr.V).Name == "Err" {
		fmt.Fprintf(in.stderr, "%s: %s\n", e.Name, in.errorMessage(vr.Payload[0]))
		return false
	}
	return true
}
