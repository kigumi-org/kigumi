package llgen

import (
	"strings"

	"kigumi/internal/mir"
)

// scalarCopyLike handles OpAlias/OpShare/OpMove when at least one side is
// natively stored. Retain-then-release around a register read is a no-op
// here and is skipped: these types have no identity beyond their value,
// and classifyScalars refuses any local ever borrowed from.
func (e *emitter) scalarCopyLike(sb *strings.Builder, f *mir.Func, in mir.Inst) bool {
	_, dstNative := e.scalar[in.Dst]
	srcInfo, srcNative := e.scalar[in.Args[0]]
	if !dstNative && !srcNative {
		return false
	}
	switch {
	case dstNative && srcNative:
		e.storeNative(sb, in.Dst, e.loadNative(sb, in.Args[0]))
	case dstNative && !srcNative:
		srcTy, ok := e.scalarType(f.Locals[in.Args[0]].Type)
		if !ok {
			return false
		}
		p := e.load(sb, in.Args[0])
		v := e.unboxScalar(sb, srcTy, p)
		if in.Op == mir.OpMove {
			e.irb(sb).Call(rtRelease, vptr(p))
			if f.Locals[in.Args[0]].Ent != 0 {
				e.store(sb, in.Args[0], "null")
			}
		}
		e.storeNative(sb, in.Dst, v)
	case !dstNative && srcNative:
		nk := e.numKind(f.Locals[in.Dst].Type)
		boxed := e.boxNative(sb, scalarInfo{Ty: srcInfo.Ty, Nk: nk}, e.loadNative(sb, in.Args[0]))
		e.store(sb, in.Dst, boxed)
	}
	return true
}
