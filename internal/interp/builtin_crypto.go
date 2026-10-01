package interp

import "kigumi/internal/syntax"

// registerCrypto covers std/crypto/*. The interpreter has no timing
// guarantee to keep (subtle.kg's doc comment says so); this mirrors the
// algorithm for correctness only.
func registerCrypto(in *Interp) {
	in.def("std/crypto/subtle.constantTimeEq", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		x := deref(a[0]).(Bytes)
		y := deref(a[1]).(Bytes)
		if len(x) != len(y) {
			return Bool(false), nil
		}
		var diff byte
		for i := range x {
			diff |= x[i] ^ y[i]
		}
		return Bool(diff == 0), nil
	})
}
