package interp

import (
	"crypto/rand"
	"encoding/binary"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// std/entropy's real body reaches getrandom(2) through extern(C), which the
// interpreter cannot call; crypto/rand gives the interpreter the same
// guarantee.
func registerEntropyAccel(in *Interp) {
	e := "std/entropy."
	in.hostAccel(e+"Entropy.bytes", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		size := deref(a[1]).(Int).V
		if size < 0 {
			return mkErr(in, fr.retType(n), "out of memory"), nil
		}
		buf := make([]byte, size)
		if _, err := rand.Read(buf); err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		return mkOk(in, fr.retType(n), Bytes(buf)), nil
	})
	in.hostAccel(e+"Entropy.u64", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		v := binary.BigEndian.Uint64(buf[:])
		return mkOk(in, fr.retType(n), Int{V: int64(v), T: sem.TyU64}), nil
	})
}
