package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// The interpreter keeps CValue contents in Go: a handle indexes the value,
// and a pointer to one is the opaque handle, so readRecord and writeRecord
// see what C would.
func registerFFIRecords(in *Interp) {
	p := "std/ffi."
	handleOf := func(v Value) int64 { return deref(v).(*Record).Fields[0].(Int).V }
	ptrHandle := func(fr *frame, n syntax.NodeID, v Value) int64 {
		h, ok := deref(v).(*Opaque).Data.(int64)
		if !ok {
			fr.panicAt(n, "this pointer does not address a CValue; run the program with `kigumi build`")
		}
		return h
	}
	in.def(p+"sizeOf", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		size, _ := in.cLayout(fr.in.r.Types.Node(fr.typeOfArg(n, 0)).Elem)
		return Int{V: size, T: sem.TyUsize}, nil
	})
	in.def(p+"alignOf", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		_, align := in.cLayout(fr.in.r.Types.Node(fr.typeOfArg(n, 0)).Elem)
		return Int{V: align, T: sem.TyUsize}, nil
	})
	in.def(p+"CValue.new", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		in.cvalues = append(in.cvalues, cloneValue(deref(a[0])))
		h := int64(len(in.cvalues))
		return &Record{Type: fr.retType(n), Fields: []Value{Int{V: h, T: sem.TyI64}}}, nil
	})
	in.def(p+"CValue.ptr", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Ptr", handleOf(a[0])), nil
	})
	in.def(p+"CValue.get", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return cloneValue(in.cvalues[handleOf(a[0])-1]), nil
	})
	in.def(p+"CValue.set", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		in.cvalues[handleOf(a[0])-1] = cloneValue(deref(a[1]))
		return Unit{}, nil
	})
	in.def(p+"CValue.drop", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return Unit{}, nil
	})
	in.def(p+"readRecord", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return cloneValue(in.cvalues[ptrHandle(fr, n, a[0])-1]), nil
	})
	in.def(p+"writeRecord", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		in.cvalues[ptrHandle(fr, n, a[0])-1] = cloneValue(deref(a[1]))
		return Unit{}, nil
	})
}

// cLayout computes the C size and alignment of a layout(C) record the way
// the native runtime does: natural alignment unless packed, widened by an
// explicit align.
func (in *Interp) cLayout(t sem.TypeID) (size, align int64) {
	tt := in.r.Types
	switch {
	case tt.IsNumeric(t):
		b := int64(tt.Width(t) / 8)
		return b, b
	case t == sem.TyBool:
		return 1, 1
	case t == sem.TyChar:
		return 4, 4
	case tt.Kind(t) == sem.KPtr || tt.IsCFn(t):
		p := int64(tt.PtrBits() / 8)
		return p, p
	}
	info := in.r.TypeDecl(tt.Node(t).Ent)
	packed := info.Layout == "packed"
	var off, maxAlign int64 = 0, 1
	for _, f := range info.Fields {
		fs, fa := in.cLayout(in.r.Entity(f).Type)
		if packed {
			fa = 1
		}
		off = (off+fa-1)/fa*fa + fs
		maxAlign = max(maxAlign, fa)
	}
	maxAlign = max(maxAlign, info.Align)
	return (off + maxAlign - 1) / maxAlign * maxAlign, maxAlign
}

// typeOfArg is the checked type of the i-th argument of the call at n.
func (fr *frame) typeOfArg(n syntax.NodeID, i int) sem.TypeID {
	args := fr.t.Children(syntax.NodeID(fr.t.Nodes[n].Rhs))
	return fr.typeOf(args[i])
}
