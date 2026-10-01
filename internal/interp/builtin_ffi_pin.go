package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// Pins share the CValue table: the handle indexes the pinned value and the
// userdata pointer is an opaque handle, so Pin.with finds it again.
func registerFFIPins(in *Interp) {
	p := "std/ffi."
	handleOf := func(v Value) int64 { return deref(v).(*Record).Fields[0].(Int).V }
	in.def(p+"Pin.new", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		in.cvalues = append(in.cvalues, deref(a[0]))
		h := int64(len(in.cvalues))
		return &Record{Type: fr.retType(n), Fields: []Value{Int{V: h, T: sem.TyI64}}}, nil
	})
	in.def(p+"Pin.ptr", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Ptr", handleOf(a[0])), nil
	})
	in.def(p+"Pin.with", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		h, ok := deref(a[0]).(*Opaque).Data.(int64)
		if !ok {
			fr.panicAt(n, "this pointer does not address a Pin; run the program with `kigumi build`")
		}
		if in.cvalues[h-1] == nil {
			fr.panicAt(n, "this pointer addresses a Pin that was already dropped")
		}
		return in.callValue(fr, a[1], n, []Value{&Ref{Cell: &Cell{V: in.cvalues[h-1]}}})
	})
	in.def(p+"Pin.drop", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		h := handleOf(a[0])
		v := in.cvalues[h-1]
		in.cvalues[h-1] = nil
		fr.dropValue(v)
		return Unit{}, nil
	})
	in.def(p+"Pin.release", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Ptr", handleOf(a[0])), nil
	})
	in.def(p+"Pin.reclaim", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		h, ok := deref(a[0]).(*Opaque).Data.(int64)
		if !ok {
			fr.panicAt(n, "this pointer does not address a Pin; run the program with `kigumi build`")
		}
		if in.cvalues[h-1] == nil {
			fr.panicAt(n, "this pointer addresses a Pin that was already dropped")
		}
		return &Record{Type: fr.retType(n), Fields: []Value{Int{V: h, T: sem.TyI64}}}, nil
	})
}
