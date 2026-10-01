package interp

import "kigumi/internal/syntax"

// The interpreter has no C ABI: pointers exist only as null, and anything
// that would touch foreign memory stops with a pointer to `kigumi build`.
func registerFFI(in *Interp) {
	p := "std/ffi."
	in.def(p+"null", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Ptr", nil), nil
	})
	in.def(p+"isNull", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return Bool(deref(a[0]).(*Opaque).Data == nil), nil
	})
	in.def(p+"toConst", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return deref(a[0]), nil
	})
	in.def(p+"toMut", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return deref(a[0]), nil
	})
	in.def(p+"cast", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return deref(a[0]), nil
	})
	registerFFIRecords(in)
	registerFFIPins(in)
	for _, name := range []string{"CString.new", "CString.ptr", "CString.drop", "offset", "string", "stringChecked", "stringLossy", "readU8", "readI32", "readI64", "readF64", "writeU8", "writeI32", "writeI64", "writeF64", "bytesFrom", "bytesPtr", "readPtr", "writePtr"} {
		key := p + name
		in.def(key, func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
			fr.panicAt(n, "`%s` needs foreign memory; run this program with `kigumi build`", key)
			return nil, nil
		})
	}
}
