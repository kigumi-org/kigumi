package interp

import (
	"kigumi/internal/syntax"
)

// registerOption implements Option[T].take: it cannot be a Kigumi body
// because a body has no way to replace an ADT value through `mut self`.
func registerOption(in *Interp) {
	in.def("std/prelude.Option.take", func(in *Interp, fr *frame, n syntax.NodeID, args []Value) (Value, *ctrl) {
		v := deref(args[0]).(*Variant)
		if in.r.Entity(v.V).Name != "Some" {
			return mkNone(in, fr.retType(n)), nil
		}
		payload := v.Payload[0]
		v.V = in.variantNamed(in.r.Types.OptionEnt(), "None")
		v.Payload = nil
		return mkSome(in, fr.retType(n), payload), nil
	})
}
