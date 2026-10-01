package vm

import (
	"math"
	"math/bits"
)

func bitsOf(nk int) int    { return nk & 255 }
func isSigned(nk int) bool { return nk>>8&1 != 0 }

func minOf(nk int) int64 {
	if !isSigned(nk) {
		return 0
	}
	if b := bitsOf(nk); b < 64 {
		return -(int64(1) << (b - 1))
	}
	return math.MinInt64
}

func maxOf(nk int) uint64 {
	b := bitsOf(nk)
	if isSigned(nk) {
		if b >= 64 {
			return math.MaxInt64
		}
		return uint64(1)<<(b-1) - 1
	}
	if b >= 64 {
		return math.MaxUint64
	}
	return uint64(1)<<b - 1
}

func truncateTo(v int64, nk int) int64 {
	b := bitsOf(nk)
	if b >= 64 {
		return v
	}
	mask := int64(1)<<b - 1
	v &= mask
	if isSigned(nk) && v&(int64(1)<<(b-1)) != 0 {
		v -= int64(1) << b
	}
	return v
}

// checked aborts on overflow of the destination width, like the runtime.
func (m *Machine) checked(v int64, nk int, over bool) *obj {
	if !over {
		if isSigned(nk) {
			over = v < minOf(nk) || v > int64(maxOf(nk))
		} else if bitsOf(nk) < 64 {
			over = v < 0 || uint64(v) > maxOf(nk)
		}
	}
	if over {
		m.abort("integer overflow")
	}
	return mkInt(v, nk)
}

func (m *Machine) binop(op string, a, b *obj) *obj {
	a, b = deref(a), deref(b)
	switch op {
	case "==":
		return mkBool(m.equal(a, b))
	case "!=":
		return mkBool(!m.equal(a, b))
	case "..":
		return mkRecord(m.lookup("std/prelude", "Range"), []*obj{retain(a), retain(b), mkBool(false)})
	case "..=":
		return mkRecord(m.lookup("std/prelude", "Range"), []*obj{retain(a), retain(b), mkBool(true)})
	}
	if a.k == kFloat {
		x, y := a.f, b.f
		switch op {
		case "<":
			return mkBool(x < y)
		case "<=":
			return mkBool(x <= y)
		case ">":
			return mkBool(x > y)
		case ">=":
			return mkBool(x >= y)
		case "+":
			return mkFloat(x+y, a.nk)
		case "-":
			return mkFloat(x-y, a.nk)
		case "*":
			return mkFloat(x*y, a.nk)
		case "/":
			return mkFloat(x/y, a.nk)
		}
	}
	switch op {
	case "<":
		return mkBool(compare(a, b) < 0)
	case "<=":
		return mkBool(compare(a, b) <= 0)
	case ">":
		return mkBool(compare(a, b) > 0)
	case ">=":
		return mkBool(compare(a, b) >= 0)
	}
	if a.k != kInt {
		m.abort("unsupported operands")
	}
	x, y, nk := a.i, b.i, a.nk
	sg := isSigned(nk)
	switch op {
	case "+":
		r := x + y
		over := sg && ((x > 0 && y > 0 && r < 0) || (x < 0 && y < 0 && r >= 0)) || !sg && bitsOf(nk) == 64 && uint64(r) < uint64(x)
		return m.checked(r, nk, over)
	case "-":
		r := x - y
		over := sg && ((x >= 0 && y < 0 && r < 0) || (x < 0 && y > 0 && r >= 0)) || !sg && uint64(x) < uint64(y)
		return m.checked(r, nk, over)
	case "*":
		if sg {
			hi, lo := bits.Mul64(uint64(x), uint64(y))
			r := int64(lo)
			over := (x != 0 && r/x != y) || (x == -1 && y == math.MinInt64) || (y == -1 && x == math.MinInt64)
			_ = hi
			return m.checked(r, nk, over)
		}
		hi, lo := bits.Mul64(uint64(x), uint64(y))
		return m.checked(int64(lo), nk, hi != 0)
	case "/", "%":
		if y == 0 {
			m.abort("division by zero")
		}
		if sg && y == -1 && x == minOf(nk) {
			m.abort("integer overflow")
		}
		if sg {
			if op == "/" {
				return mkInt(x/y, nk)
			}
			return mkInt(x%y, nk)
		}
		if op == "/" {
			return mkInt(int64(uint64(x)/uint64(y)), nk)
		}
		return mkInt(int64(uint64(x)%uint64(y)), nk)
	case "&":
		return mkInt(x&y, nk)
	case "|":
		return mkInt(x|y, nk)
	case "^":
		return m.checked(x^y, nk, false)
	case "<<":
		if y < 0 || y >= int64(bitsOf(nk)) {
			m.abort("shift count out of range")
		}
		return m.checked(x<<uint(y), nk, false)
	case ">>":
		if y < 0 || y >= int64(bitsOf(nk)) {
			m.abort("shift count out of range")
		}
		if sg {
			return mkInt(x>>uint(y), nk)
		}
		return mkInt(int64(uint64(x)>>uint(y)), nk)
	}
	m.abort("unsupported operator " + op)
	return nil
}

func (m *Machine) unop(op string, a *obj) *obj {
	a = deref(a)
	switch op[0] {
	case '!':
		return mkBool(!a.b)
	case '-':
		if a.k == kFloat {
			return mkFloat(-a.f, a.nk)
		}
		return m.checked(-a.i, a.nk, a.i == minOf(a.nk))
	case '~':
		return mkInt(truncateTo(^a.i, a.nk), a.nk)
	}
	m.abort("unsupported unary " + op)
	return nil
}

func (m *Machine) cast(a *obj, nk int) *obj {
	a = deref(a)
	if nk&512 != 0 {
		if a.k == kFloat {
			return mkFloat(a.f, nk)
		}
		return mkFloat(float64(a.i), nk)
	}
	return mkInt(a.i, nk)
}
