package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// registerError implements std/error.as[E]: `e is E(x)` doesn't type-check
// over a type parameter (E101), so this downcast compares box
// identity the way `is` does for a concrete type, looking through any
// Context wrapping.
func registerError(in *Interp) {
	in.def("std/error.as", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		opt, _ := in.r.Types.IsOption(fr.retType(n))
		want := in.r.Types.Node(opt).Ent
		ctxEnt := in.r.LangItem("Context")
		cur := deref(a[0])
		for {
			box, ok := cur.(*Box)
			if !ok {
				return mkNone(in, fr.retType(n)), nil
			}
			dn := in.r.Types.Node(box.Dyn)
			if dn.Kind != sem.KNamed {
				return mkNone(in, fr.retType(n)), nil
			}
			if dn.Ent == want {
				return mkSome(in, fr.retType(n), cloneValue(box.V)), nil
			}
			if ctxEnt == 0 || dn.Ent != ctxEnt {
				return mkNone(in, fr.retType(n)), nil
			}
			wrapped := in.r.Field(in.r.FindField(ctxEnt, "wrapped")).Index
			cur = deref(box.V.(*Record).Fields[wrapped])
		}
	})
}
