package interp

import "kigumi/internal/sem"

// witnessValues produces the witness arguments a call passes after its
// ordinary ones (sem.CallInfo.Passes).
func (fr *frame) witnessValues(passes []sem.WitnessArg) []Value {
	var out []Value
	for _, w := range passes {
		out = append(out, fr.witnessValue(w))
	}
	return out
}

func (fr *frame) witnessValue(w sem.WitnessArg) Value {
	switch w.Kind {
	case sem.WitnessLocal:
		return fr.cell(w.Ent).V
	case sem.WitnessFn:
		return &FnItem{Fn: w.Ent}
	case sem.WitnessBind:
		return &Partial{Fn: w.Ent, Bound: append(fr.witnessValues(w.Args), fr.constValues(w.ConstArgs)...)}
	}
	return Unit{}
}
