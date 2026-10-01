package interp

import "kigumi/internal/syntax"

// std/dl needs the dynamic loader; the interpreter points at `kigumi build`.
func registerDL(in *Interp) {
	for _, name := range []string{"Symbol.call"} {
		key := "std/dl." + name
		in.def(key, func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
			fr.panicAt(n, "`%s` needs the dynamic loader; run this program with `kigumi build`", key)
			return nil, nil
		})
	}
}
