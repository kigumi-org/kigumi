package vm

import "kigumi/internal/sem"

// dynType recovers a type for member lookup from a runtime value.
func (m *Machine) dynType(v *obj) sem.TypeID {
	tt := m.r.Types
	switch v.k {
	case kRecord, kVariant:
		return tt.Named(v.ent, nil)
	case kInt:
		return m.intType(v.nk)
	case kFloat:
		if v.nk&255 == 32 {
			return sem.TyF32
		}
		return sem.TyF64
	case kBool:
		return sem.TyBool
	case kStr:
		return sem.TyString
	case kBytes:
		return sem.TyBytes
	case kChar:
		return sem.TyChar
	case kArray:
		return tt.Named(tt.ArrayEnt(), nil)
	}
	return sem.TyUnit
}

func (m *Machine) intType(nk int) sem.TypeID {
	signed := nk&256 != 0
	switch nk & 255 {
	case 8:
		if signed {
			return sem.TyI8
		}
		return sem.TyU8
	case 16:
		if signed {
			return sem.TyI16
		}
		return sem.TyU16
	case 32:
		if signed {
			return sem.TyI32
		}
		return sem.TyU32
	}
	if signed {
		return sem.TyI64
	}
	return sem.TyU64
}

func (m *Machine) errorMessage(e *obj) *obj {
	return m.callMethod(e, "message", nil)
}
