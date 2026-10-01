package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
	"math"
)

func bitsOf(in *Interp, t sem.TypeID) int { return in.r.Types.Width(t) }

func minOf(in *Interp, t sem.TypeID) int64 {
	if !in.r.Types.IsSigned(t) {
		return 0
	}
	if b := bitsOf(in, t); b < 64 {
		return -1 << (b - 1)
	}
	return math.MinInt64
}

func maxOf(in *Interp, t sem.TypeID) uint64 {
	b := bitsOf(in, t)
	if in.r.Types.IsSigned(t) {
		if b >= 64 {
			return math.MaxInt64
		}
		return 1<<(b-1) - 1
	}
	if b >= 64 {
		return ^uint64(0)
	}
	return 1<<b - 1
}

// mkFloat builds a float of type t; an f32 keeps only single precision, so
// each operation rounds the way IEEE single does.
func mkFloat(in *Interp, v float64, t sem.TypeID) Float {
	if in != nil && in.r.Types.Width(t) == 32 {
		v = float64(float32(v))
	}
	return Float{V: v, T: t}
}

// truncate wraps a bit pattern to the width of t (bitwise operators).
func truncate(v int64, t sem.TypeID, in *Interp) int64 {
	b := bitsOf(in, t)
	if b == 64 {
		return v
	}
	mask := int64(1)<<b - 1
	v &= mask
	if in.r.Types.IsSigned(t) && v&(1<<(b-1)) != 0 {
		v -= 1 << b
	}
	return v
}

func (fr *frame) checkedInt(n syntax.NodeID, v int64, t sem.TypeID, overflow bool) Value {
	in := fr.in
	if !overflow {
		if in.r.Types.IsSigned(t) {
			overflow = v < minOf(in, t) || v > int64(maxOf(in, t))
		} else if bitsOf(in, t) < 64 {
			overflow = v < 0 || uint64(v) > maxOf(in, t)
		}
	}
	if overflow {
		fr.panicAt(n, "integer overflow")
	}
	return Int{V: v, T: t}
}

func (fr *frame) binaryOp(n syntax.NodeID, op token.Kind, l, r Value) Value {
	l, r = deref(l), deref(r)
	switch op {
	case token.EqEq:
		return Bool(valueEqual(l, r))
	case token.NotEq:
		return Bool(!valueEqual(l, r))
	case token.DotDot:
		return &Record{Type: fr.typeOf(n), Fields: []Value{l, r, Bool(false)}}
	case token.DotDotEq:
		return &Record{Type: fr.typeOf(n), Fields: []Value{l, r, Bool(true)}}
	}
	switch x := l.(type) {
	case Int:
		return fr.intOp(n, op, x, r.(Int))
	case Float:
		return floatOp(fr.in, op, x, r.(Float))
	case Str:
		return Bool(compareOp(op, cmpStr(string(x), string(r.(Str)))))
	case Char:
		return Bool(compareOp(op, cmpInt(int64(x), int64(r.(Char)))))
	case Bool:
		return Bool(compareOp(op, cmpInt(b2i(bool(x)), b2i(bool(r.(Bool))))))
	case Unit:
		return Bool(compareOp(op, 0))
	}
	fr.panicAt(n, "unsupported operands")
	return nil
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpStr(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func compareOp(op token.Kind, c int) bool {
	switch op {
	case token.Lt:
		return c < 0
	case token.LtEq:
		return c <= 0
	case token.Gt:
		return c > 0
	case token.GtEq:
		return c >= 0
	}
	return false
}
