package interp

import (
	"strconv"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

func str(v Value) string { return string(deref(v).(Str)) }

func registerText(in *Interp) {
	p := "std/prelude.String."
	in.def(p+"bytes", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return Bytes([]byte(str(a[0]))), nil
	})
	in.def("std/prelude.Bytes.get", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		b := deref(a[0]).(Bytes)
		i := deref(a[1]).(Int).V
		if i < 0 || i >= int64(len(b)) {
			return mkNone(in, fr.retType(n)), nil
		}
		return mkSome(in, fr.retType(n), Int{V: int64(b[i]), T: sem.TyU8}), nil
	})
	in.def("std/prelude.Bytes.slice", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		b := deref(a[0]).(Bytes)
		start, end := int(deref(a[1]).(Int).V), int(deref(a[2]).(Int).V)
		if start < 0 || start > end || end > len(b) {
			return mkNone(in, fr.retType(n)), nil
		}
		return mkSome(in, fr.retType(n), append(Bytes{}, b[start:end]...)), nil
	})
	in.def("std/prelude.Bytes.zeros", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		v := deref(a[0]).(Int).V
		if v < 0 {
			fr.panicAt(n, "out of memory")
		}
		return Bytes(make([]byte, v)), nil
	})
	in.def("std/prelude.Bytes.fill", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		value := byte(deref(a[0]).(Int).V)
		v := deref(a[1]).(Int).V
		if v < 0 {
			fr.panicAt(n, "out of memory")
		}
		out := make([]byte, v)
		for i := range out {
			out[i] = value
		}
		return Bytes(out), nil
	})
	in.def("std/prelude.Bytes.fromArray", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		var out []byte
		for _, e := range deref(a[0]).(*Array).Elems {
			out = append(out, byte(deref(e).(Int).V))
		}
		return Bytes(out), nil
	})
	// Go's append never refuses a real, already-in-memory input the way the
	// growBy threshold simulates a refusal for a forward reservation.
	in.def("std/prelude.Bytes.tryFromArray", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		var out []byte
		for _, e := range deref(a[0]).(*Array).Elems {
			out = append(out, byte(deref(e).(Int).V))
		}
		return mkOk(in, fr.retType(n), Bytes(out)), nil
	})
	in.def("std/prelude.Bytes.concat", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		self, other := deref(a[0]).(Bytes), deref(a[1]).(Bytes)
		out := make(Bytes, 0, len(self)+len(other))
		out = append(out, self...)
		out = append(out, other...)
		return out, nil
	})
	in.def("std/prelude.Bytes.tryConcat", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		self, other := deref(a[0]).(Bytes), deref(a[1]).(Bytes)
		out := make(Bytes, 0, len(self)+len(other))
		out = append(out, self...)
		out = append(out, other...)
		return mkOk(in, fr.retType(n), out), nil
	})
	in.def("std/text.parseFloat", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		s := str(a[0])
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || s == "" || s[0] == ' ' || s[0] == '+' {
			return mkNone(in, fr.retType(n)), nil
		}
		return mkSome(in, fr.retType(n), mkFloat(in, f, sem.TyF64)), nil
	})
	in.def("std/text.fromChar", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return Str(string(rune(deref(a[0]).(Char)))), nil
	})
}

func registerArrayExtra(in *Interp) {
}
