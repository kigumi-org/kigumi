package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

func flt(v Value) float64 { return deref(v).(Float).V }

func f64(v float64) Value { return Float{V: v, T: sem.TyF64} }

func registerHostExtra(in *Interp) {
	o := "std/os."
	in.def(o+"Host.stdin", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Stdin", nil), nil
	})
	in.def(o+"Host.stdout", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Stdout", nil), nil
	})
	in.def(o+"Args.len", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return usize(len(deref(a[0]).(*Opaque).Data.([]string))), nil
	})
	registerMathTextTime(in)
	registerNumeric(in)
	registerConversions(in)
	registerConvert(in)
}

func registerConversions(in *Interp) {
	in.def("std/array.Array.clone", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return cloneValue(deref(a[0])), nil
	})
}
