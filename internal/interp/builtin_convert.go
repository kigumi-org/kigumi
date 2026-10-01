package interp

import (
	"math"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

type intKind struct {
	name   string
	bits   int
	signed bool
	method string
	t      sem.TypeID
}

var intKinds = []intKind{
	{"i8", 8, true, "I8", sem.TyI8},
	{"i16", 16, true, "I16", sem.TyI16},
	{"i32", 32, true, "I32", sem.TyI32},
	{"i64", 64, true, "Int", sem.TyI64},
	{"isize", 64, true, "Isize", sem.TyIsize},
	{"u8", 8, false, "U8", sem.TyU8},
	{"u16", 16, false, "U16", sem.TyU16},
	{"u32", 32, false, "U32", sem.TyU32},
	{"u64", 64, false, "U64", sem.TyU64},
	{"usize", 64, false, "Usize", sem.TyUsize},
}

// fitsAlways mirrors std/prelude/convert.kg: same signedness and no narrower, or
// unsigned into a strictly wider signed type.
func fitsAlways(src, dst intKind) bool {
	return src.signed == dst.signed && dst.bits >= src.bits || dst.signed && !src.signed && dst.bits > src.bits
}

// inRange checks a source value (held as int64; unsigned 64 bit values wrap) against the destination kind.
func inRange(v int64, src, dst intKind) bool {
	if !src.signed && v < 0 && dst.bits < 64 {
		return false
	}
	if dst.bits == 64 {
		if dst.signed {
			return src.signed || v >= 0
		}
		return !src.signed || v >= 0
	}
	if dst.signed {
		return v >= -(int64(1)<<(dst.bits-1)) && v < int64(1)<<(dst.bits-1)
	}
	return v >= 0 && v < int64(1)<<dst.bits
}

// registerConvert supplies every conversion declared in std/prelude/convert.kg.
func registerConvert(in *Interp) {
	p := "std/prelude."
	for _, src := range intKinds {
		src := src
		for _, dst := range intKinds {
			dst := dst
			if src.name == dst.name {
				continue
			}
			in.def(p+src.name+".to"+dst.method, func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
				v := deref(a[0]).(Int).V
				if fitsAlways(src, dst) {
					return Int{V: v, T: dst.t}, nil
				}
				if !inRange(v, src, dst) {
					return mkNone(in, fr.retType(n)), nil
				}
				return mkSome(in, fr.retType(n), Int{V: v, T: dst.t}), nil
			})
		}
		in.def(p+src.name+".toFloat", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
			return mkFloat(in, intToFloat(deref(a[0]).(Int).V, src.signed), sem.TyF64), nil
		})
		in.def(p+src.name+".toF32", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
			return mkFloat(in, intToFloat(deref(a[0]).(Int).V, src.signed), sem.TyF32), nil
		})
	}
	floatToInt := func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		f := math.Trunc(deref(a[0]).(Float).V)
		if math.IsNaN(f) || f < -9223372036854775808.0 || f >= 9223372036854775808.0 {
			return mkNone(in, fr.retType(n)), nil
		}
		return mkSome(in, fr.retType(n), Int{V: int64(f), T: sem.TyI64}), nil
	}
	in.def(p+"f64.toInt", floatToInt)
	in.def(p+"f32.toInt", floatToInt)
	in.def(p+"f64.toF32", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return mkFloat(in, deref(a[0]).(Float).V, sem.TyF32), nil
	})
	in.def(p+"f32.toF64", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return mkFloat(in, deref(a[0]).(Float).V, sem.TyF64), nil
	})
	in.def(p+"Char.toInt", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return Int{V: int64(deref(a[0]).(Char)), T: sem.TyI64}, nil
	})
	in.def(p+"Char.fromInt", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		v := deref(a[0]).(Int).V
		if v < 0 || v > 0x10FFFF || v >= 0xD800 && v <= 0xDFFF {
			return mkNone(in, fr.retType(n)), nil
		}
		return mkSome(in, fr.retType(n), Char(rune(v))), nil
	})
	in.def(p+"u8.toChar", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return Char(rune(deref(a[0]).(Int).V)), nil
	})
}

func intToFloat(v int64, signed bool) float64 {
	if !signed && v < 0 {
		return float64(uint64(v))
	}
	return float64(v)
}
