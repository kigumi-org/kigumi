package sem

import "kigumi/internal/syntax"

func (r *Result) constFits(f FileID, n syntax.NodeID, v constValue, typ TypeID) bool {
	tt := r.Types
	switch {
	case tt.IsInteger(typ) && v.Kind == constInt:
		if !r.intFits(v.Int, typ) {
			r.errAt(f, n, cLiteralOutOfRange, v.Int.String(), typ)
			return false
		}
	case tt.IsInteger(typ) && v.Kind == constFloat:
		r.errAt(f, n, cLiteralFloatIntoInt, v.Float.Text('g', -1), typ)
		return false
	case tt.IsFloat(typ) && v.Kind == constInt:
		if !intExactAsFloat(v.Int, tt.Width(typ)) {
			r.errAt(f, n, cLiteralOutOfRange, v.Int.String(), typ)
			return false
		}
	case tt.IsFloat(typ) && v.Kind == constFloat:
	case typ == r.untypedOf(v):
	case typ == TyPoison:
	default:
		r.errAt(f, n, cTypeMismatch, typ, r.untypedOf(v))
		return false
	}
	return true
}
