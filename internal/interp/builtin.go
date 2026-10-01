package interp

import (
	"unicode/utf8"

	"kigumi/internal/hashkey"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

func (in *Interp) def(key string, f builtinFn) { in.builtins[key] = f }

// retType is the declared result type of the called function, for
// constructing Option/Result values.
func (fr *frame) retType(n syntax.NodeID) sem.TypeID { return fr.typeOf(n) }

func (in *Interp) intrinsic(fr *frame, n syntax.NodeID, name string, args []Value) (Value, *ctrl) {
	switch name {
	case "print":
		in.stdout.Write([]byte(display(args[0], in) + "\n"))
	case "eprint":
		in.stderr.Write([]byte(display(args[0], in) + "\n"))
	case "panic":
		if len(args) > 0 {
			fr.panicAt(n, "%s", string(deref(args[0]).(Str)))
		}
		fr.panicAt(n, "explicit panic")
	}
	return Unit{}, nil
}

func usize(v int) Value { return Int{V: int64(v), T: sem.TyUsize} }

func registerPrelude(in *Interp) {
	p := "std/prelude."
	in.def(p+"String.len", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return usize(len(deref(a[0]).(Str))), nil
	})
	in.def(p+"String.toBytes", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return Bytes([]byte(deref(a[0]).(Str))), nil
	})
	in.def(p+"String.fromBytes", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		b := deref(a[0]).(Bytes)
		if !utf8.Valid(b) {
			return mkErr(in, fr.retType(n), "invalid UTF-8"), nil
		}
		return mkOk(in, fr.retType(n), Str(string(b))), nil
	})
	// Same check as fromBytes; Go's allocator has no refusal to simulate here.
	in.def(p+"String.tryFromBytes", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		b := deref(a[0]).(Bytes)
		if !utf8.Valid(b) {
			return mkErr(in, fr.retType(n), "invalid UTF-8"), nil
		}
		return mkOk(in, fr.retType(n), Str(string(b))), nil
	})
	in.def(p+"String.equals", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return Bool(valueEqual(deref(a[0]), deref(a[1]))), nil
	})
	in.def(p+"Bytes.len", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return usize(len(deref(a[0]).(Bytes))), nil
	})
	ordering := func(c int, fr *frame, n syntax.NodeID) Value {
		names := []string{"Less", "Equal", "Greater"}
		ent := in.r.LangItem("Ordering")
		if ent == 0 {
			ent = in.r.PackageMember(in.r.PackageByPath("std/prelude"), "Ordering")
		}
		return &Variant{Type: fr.retType(n), V: in.variantNamed(ent, names[c+1])}
	}
	for _, owner := range []string{"String", "i64", "i32", "u8", "usize", "i8", "i16", "isize", "u16", "u32", "u64", "f64", "f32", "Bool", "Char", "Bytes", "Unit"} {
		isFloat := owner == "f64" || owner == "f32"
		in.def(p+owner+".compareTo", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
			x, y := deref(a[0]), deref(a[1])
			if isFloat {
				return ordering(hashkey.TotalCompareFloat64(x.(Float).V, y.(Float).V), fr, n), nil
			}
			return ordering(compareValues(x, y), fr, n), nil
		})
		in.def(p+owner+".equals", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
			x, y := deref(a[0]), deref(a[1])
			if isFloat {
				return Bool(hashkey.TotalEqualFloat64(x.(Float).V, y.(Float).V)), nil
			}
			return Bool(valueEqual(x, y)), nil
		})
		in.def(p+owner+".hash", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
			return Int{V: int64(hashValue(deref(a[0]), in)), T: sem.TyU64}, nil
		})
	}
	in.def(p+"debug", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return Str(display(deref(a[0]), in)), nil
	})
}

func compareValues(a, b Value) int {
	switch x := a.(type) {
	case Unit:
		return 0
	case Int:
		return cmpInt(x.V, b.(Int).V)
	case Str:
		return cmpStr(string(x), string(b.(Str)))
	case Float:
		switch {
		case x.V < b.(Float).V:
			return -1
		case x.V > b.(Float).V:
			return 1
		}
	}
	return 0
}
