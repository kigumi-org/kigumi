package interp

import (
	"kigumi/internal/syntax"
)

func registerCollections(in *Interp) {
	a := "std/array."
	in.def(a+"Array.empty", func(in *Interp, fr *frame, n syntax.NodeID, args []Value) (Value, *ctrl) {
		return &Array{}, nil
	})
	in.def(a+"Array.of", func(in *Interp, fr *frame, n syntax.NodeID, args []Value) (Value, *ctrl) {
		return cloneValue(deref(args[0])), nil
	})
	in.def(a+"Array.push", func(in *Interp, fr *frame, n syntax.NodeID, args []Value) (Value, *ctrl) {
		arr := deref(args[0]).(*Array)
		arr.Elems = append(arr.Elems, args[1])
		return Unit{}, nil
	})
	in.def(a+"Array.growBy", func(in *Interp, fr *frame, n syntax.NodeID, args []Value) (Value, *ctrl) {
		// Go cannot report a refused allocation; an absurd request stands in for one.
		return Bool(uint64(deref(args[1]).(Int).V) < 1<<40), nil
	})
	in.def(a+"Array.with", func(in *Interp, fr *frame, n syntax.NodeID, args []Value) (Value, *ctrl) {
		arr := deref(args[0]).(*Array)
		i := deref(args[1]).(Int).V
		if i < 0 || int(i) >= len(arr.Elems) {
			return mkNone(in, fr.retType(n)), nil
		}
		r, c := in.callValue(fr, args[2], n, []Value{&Ref{Cell: &Cell{V: arr.Elems[i]}}})
		if c != nil {
			return nil, c
		}
		return mkSome(in, fr.retType(n), r), nil
	})
	in.def(a+"Array.withMut", func(in *Interp, fr *frame, n syntax.NodeID, args []Value) (Value, *ctrl) {
		arr := deref(args[0]).(*Array)
		i := deref(args[1]).(Int).V
		if i < 0 || int(i) >= len(arr.Elems) {
			return mkNone(in, fr.retType(n)), nil
		}
		// Path-indexed, unlike with's snapshot cell, so writes through the
		// &mut land back in arr.Elems[i] even for non-pointer T.
		r, c := in.callValue(fr, args[2], n, []Value{&Ref{Cell: &Cell{V: arr}, Path: []int{int(i)}}})
		if c != nil {
			return nil, c
		}
		return mkSome(in, fr.retType(n), r), nil
	})
	in.def(a+"Array.takeAt", func(in *Interp, fr *frame, n syntax.NodeID, args []Value) (Value, *ctrl) {
		arr := deref(args[0]).(*Array)
		i := deref(args[1]).(Int).V
		if i < 0 || int(i) >= len(arr.Elems) {
			return mkNone(in, fr.retType(n)), nil
		}
		x := arr.Elems[i]
		arr.Elems = append(arr.Elems[:i], arr.Elems[i+1:]...)
		return mkSome(in, fr.retType(n), x), nil
	})
	in.def(a+"Array.len", func(in *Interp, fr *frame, n syntax.NodeID, args []Value) (Value, *ctrl) {
		return usize(len(deref(args[0]).(*Array).Elems)), nil
	})
	in.def(a+"Array.get", func(in *Interp, fr *frame, n syntax.NodeID, args []Value) (Value, *ctrl) {
		arr := deref(args[0]).(*Array)
		i := deref(args[1]).(Int).V
		if i < 0 || int(i) >= len(arr.Elems) {
			return mkNone(in, fr.retType(n)), nil
		}
		return mkSome(in, fr.retType(n), cloneValue(arr.Elems[i])), nil
	})
}
