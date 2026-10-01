package vm

import (
	"strconv"
	"strings"

	"kigumi/internal/hashkey"
)

func (m *Machine) display(v *obj) string {
	var sb strings.Builder
	m.displayInto(&sb, v)
	return sb.String()
}

func (m *Machine) displayInto(sb *strings.Builder, v *obj) {
	m.displayIntoCanon(sb, v, false)
}

// hashDisplay renders v the way displayInto does, except Float canonicalizes
// -0.0 to 0.0 so hash() agrees with Eq.
func (m *Machine) hashDisplay(v *obj) string {
	var sb strings.Builder
	m.displayIntoCanon(&sb, v, true)
	return sb.String()
}

func (m *Machine) displayIntoCanon(sb *strings.Builder, v *obj, canon bool) {
	v = deref(v)
	switch v.k {
	case kInt:
		if v.nk&256 != 0 {
			sb.WriteString(strconv.FormatInt(v.i, 10))
		} else {
			sb.WriteString(strconv.FormatUint(uint64(v.i), 10))
		}
	case kFloat:
		f := v.f
		if canon {
			f = hashkey.CanonicalFloat64(f)
		}
		bits := 64
		if v.nk&255 == 32 {
			bits = 32
		}
		sb.WriteString(strconv.FormatFloat(f, 'g', -1, bits))
	case kBool:
		sb.WriteString(strconv.FormatBool(v.b))
	case kChar:
		sb.WriteRune(v.c)
	case kStr, kBytes:
		sb.Write(v.s)
	case kUnit:
		sb.WriteString("()")
	case kVariant:
		sb.WriteString(m.r.Entity(v.ent).Name)
		if len(v.fields) > 0 {
			sb.WriteString("(")
			for i, p := range v.fields {
				if i > 0 {
					sb.WriteString(", ")
				}
				m.displayIntoCanon(sb, p, canon)
			}
			sb.WriteString(")")
		}
	case kRecord:
		sb.WriteString(m.r.Entity(v.ent).Name)
		sb.WriteString(" {")
		for i, f := range v.fields {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(" ")
			m.displayIntoCanon(sb, f, canon)
		}
		sb.WriteString(" }")
	case kArray:
		sb.WriteString("[")
		for i, e := range v.fields {
			if i > 0 {
				sb.WriteString(", ")
			}
			m.displayIntoCanon(sb, e, canon)
		}
		sb.WriteString("]")
	case kBox:
		m.displayIntoCanon(sb, v.inner, canon)
	case kOpaque:
		if v.op == "Error" || v.op == "Path" {
			sb.WriteString(v.data.(string))
		} else {
			sb.WriteString("<" + v.op + ">")
		}
	default:
		sb.WriteString("<value>")
	}
}

func (m *Machine) equal(a, b *obj) bool {
	a, b = deref(a), deref(b)
	if a.k != b.k {
		return false
	}
	switch a.k {
	case kInt:
		return a.i == b.i
	case kFloat:
		return a.f == b.f
	case kBool:
		return a.b == b.b
	case kChar:
		return a.c == b.c
	case kUnit:
		return true
	case kStr, kBytes:
		return string(a.s) == string(b.s)
	}
	// Records, ADTs and arrays compare through their `equals`.
	return a == b
}

// compare is the three-way ordering of ints, chars, bools and strings.
func compare(a, b *obj) int {
	a, b = deref(a), deref(b)
	switch a.k {
	case kInt:
		if a.nk&256 != 0 {
			return cmp3(a.i < b.i, a.i > b.i)
		}
		return cmp3(uint64(a.i) < uint64(b.i), uint64(a.i) > uint64(b.i))
	case kFloat:
		return cmp3(a.f < b.f, a.f > b.f)
	case kChar:
		return cmp3(a.c < b.c, a.c > b.c)
	case kBool:
		return cmp3(!a.b && b.b, a.b && !b.b)
	case kStr, kBytes:
		return strings.Compare(string(a.s), string(b.s))
	}
	return 0
}

func cmp3(less, greater bool) int {
	if less {
		return -1
	}
	if greater {
		return 1
	}
	return 0
}
