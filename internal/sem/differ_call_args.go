package sem

import "kigumi/internal/syntax"

// callArgsDefinitelyMismatch reports whether some argument's declared or
// literal type definitely differs from candidate g's parameter type, at any
// position, not only the one forwarded — checking only that one let an
// unrelated argument slip past what real overload resolution would already
// rule out.
func (r *Result) callArgsDefinitelyMismatch(info *FnInfo, t *syntax.Tree, args []syntax.NodeID, gParams []EntityID) bool {
	for i, a := range args {
		if i >= len(gParams) || t.Kind(a) == syntax.Spread {
			continue
		}
		target := r.Entities[gParams[i]].Type
		if p := r.paramIdent(info, t, a); p != 0 {
			if r.typesDefinitelyDiffer(r.Entities[p].Type, target) {
				return true
			}
			continue
		}
		if r.literalArgDefinitelyMismatches(t, a, target) {
			return true
		}
	}
	return false
}
