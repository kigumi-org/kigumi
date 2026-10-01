package sem

import "kigumi/internal/syntax"

// checkAbiTypes keeps C signatures to what the C ABI can carry.
func (r *Result) checkAbiTypes(f FileID, s fnSlots, params []TypeID, ret TypeID) {
	t := r.tree(f)
	i := 0
	for _, p := range t.Children(s.Params) {
		ps := param(t, p)
		if ps.Flags&syntax.FlagSelf != 0 || ps.Type == 0 {
			continue
		}
		if i < len(params) && !r.abiSafe(params[i]) {
			r.errAt(f, ps.Type, cAbiType, r.TypeString(params[i]))
		}
		i++
	}
	if s.Ret != 0 && ret != TyUnit && ret != TyNever && !r.abiSafe(ret) {
		r.errAt(f, s.Ret, cAbiType, r.TypeString(ret))
	}
}

func (r *Result) abiSafe(t TypeID) bool {
	tt := r.Types
	return t == TyPoison || tt.IsNumeric(t) || tt.Kind(t) == KPtr || tt.IsCFn(t) || r.cRecord(t)
}

// cRecord reports whether t is a user record with a C layout, which the C
// ABI moves by value.
func (r *Result) cRecord(t TypeID) bool {
	n := r.Types.Node(t)
	return n.Kind == KNamed && r.Entities[n.Ent].File != 0 && r.typeDecl(n.Ent).Layout != ""
}
