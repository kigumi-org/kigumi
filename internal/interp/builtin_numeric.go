package interp

import (
	"kigumi/internal/syntax"
)

// registerNumeric supplies the wrapping and rotate functions of the prelude.
func registerNumeric(in *Interp) {
	p := "std/prelude."
	intOf := func(v Value) int64 { return deref(v).(Int).V }
	ops := map[string]func(x, y uint64, bits int, signed bool) uint64{
		"wrappingAdd":       func(x, y uint64, _ int, _ bool) uint64 { return x + y },
		"wrappingSub":       func(x, y uint64, _ int, _ bool) uint64 { return x - y },
		"wrappingMul":       func(x, y uint64, _ int, _ bool) uint64 { return x * y },
		"wrappingShiftLeft": func(x, y uint64, bits int, _ bool) uint64 { return x << (y & uint64(bits-1)) },
		"wrappingShiftRight": func(x, y uint64, bits int, signed bool) uint64 {
			c := y & uint64(bits-1)
			if signed {
				sx := int64(x<<(64-uint(bits))) >> (64 - uint(bits))
				return uint64(sx >> c)
			}
			return x >> c
		},
		"rotateLeft":  func(x, y uint64, bits int, _ bool) uint64 { return rotate(x, int(y&uint64(bits-1)), bits) },
		"rotateRight": func(x, y uint64, bits int, _ bool) uint64 { return rotate(x, bits-int(y&uint64(bits-1)), bits) },
	}
	for _, name := range []string{"i8", "i16", "i32", "i64", "isize", "u8", "u16", "u32", "u64", "usize"} {
		for op, f := range ops {
			in.def(p+name+"."+op, func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
				t := fr.retType(n)
				bits := bitsOf(in, t)
				x := uint64(truncate(intOf(a[0]), t, in))
				if bits < 64 {
					x &= 1<<bits - 1
				}
				r := f(x, uint64(intOf(a[1])), bits, in.r.Types.IsSigned(t))
				return Int{V: truncate(int64(r), t, in), T: t}, nil
			})
		}
	}
}

// rotate turns the low bits of x left by c within a width of bits.
func rotate(x uint64, c, bits int) uint64 {
	c %= bits
	if bits < 64 {
		x &= 1<<bits - 1
	}
	if c == 0 {
		return x
	}
	return x<<c | x>>(bits-c)
}
