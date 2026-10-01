package vm

import (
	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

func (m *Machine) constant(in *mir.Inst) *obj {
	lit := in.Lit
	tt := m.r.Types
	switch {
	case in.Str != "" && tt.IsInteger(in.Type):
		var i int64
		for _, c := range in.Str {
			i = i*10 + int64(c-'0')
		}
		return mkInt(i, m.numKind(in.Type))
	case tt.IsFloat(in.Type):
		f := 0.0
		if lit.Float != nil {
			f, _ = lit.Float.Float64()
		} else if lit.Int != nil {
			f, _ = lit.Int.Float64()
		}
		return mkFloat(f, m.numKind(in.Type))
	case lit.Kind == sem.LitInt:
		var i int64
		if lit.Int != nil {
			if lit.Int.IsInt64() {
				i = lit.Int.Int64()
			} else {
				i = int64(lit.Int.Uint64())
			}
		}
		return mkInt(i, m.numKind(in.Type))
	case lit.Kind == sem.LitBool:
		return mkBool(lit.Bool)
	case lit.Kind == sem.LitString:
		return mkStr([]byte(lit.Str))
	case lit.Kind == sem.LitBytes:
		return mkBytes([]byte(lit.Str))
	case lit.Kind == sem.LitChar:
		return mkChar(lit.Char)
	}
	return unitObj
}

// numKind encodes a numeric type the runtime's way: width, 256 for
// signed, 512 for float.
func (m *Machine) numKind(t sem.TypeID) int { return mir.NumKind(m.r.Types, t) }

func (m *Machine) typeEnt(t sem.TypeID) sem.EntityID {
	if m.r.Types.Kind(t) == sem.KNamed {
		return m.r.Types.Node(t).Ent
	}
	return 0
}

func (m *Machine) field(r *obj, i int) *obj {
	r = deref(r)
	if r.k == kBox {
		r = r.inner
	}
	return retain(r.fields[i])
}

// fieldMove takes field i out of r without a retain: r's slot goes nil,
// which release already treats as a no-op, so the field is dropped once
// through whichever of r or the returned value reaches a drop first.
func (m *Machine) fieldMove(r *obj, i int) *obj {
	r = deref(r)
	if r.k == kBox {
		r = r.inner
	}
	v := r.fields[i]
	r.fields[i] = nil
	return v
}
