package llgen

import (
	"fmt"
	"strings"

	"kigumi/internal/mir"
)

// tryScalarInst emits in on native registers when e.scalar allows it with
// no box, reporting whether it fully handled in (else the caller boxes).
func (e *emitter) tryScalarInst(sb *strings.Builder, f *mir.Func, in mir.Inst) bool {
	switch in.Op {
	case mir.OpConst:
		return e.scalarConst(sb, in)
	case mir.OpBinary:
		return e.scalarBinary(sb, f, in)
	case mir.OpUnary:
		if in.Str == "cast" {
			return false
		}
		return e.scalarUnary(sb, f, in)
	case mir.OpAlias, mir.OpShare, mir.OpMove:
		return e.scalarCopyLike(sb, f, in)
	case mir.OpDrop, mir.OpRelease:
		// A value this pass never boxed has no refcount to manage.
		return e.isScalar(in.Args[0])
	}
	return false
}

func (e *emitter) isScalar(l mir.LocalID) bool {
	_, ok := e.scalar[l]
	return ok
}

func (e *emitter) loadNative(sb *strings.Builder, l mir.LocalID) Value {
	info := e.scalar[l]
	t := e.newTmp()
	fmt.Fprintf(sb, "  %s = load %s, ptr %%l%d\n", t, info.Ty, l)
	return Value{info.Ty, t}
}

func (e *emitter) storeNative(sb *strings.Builder, l mir.LocalID, v Value) {
	fmt.Fprintf(sb, "  store %s %s, ptr %%l%d\n", v.Type, v.Text, l)
}

// boxNative is the only place this pass ever allocates, at the boundaries
// the plan calls for (a call argument, a store, a return, ...).
func (e *emitter) boxNative(sb *strings.Builder, info scalarInfo, v Value) string {
	switch info.Ty {
	case TI64:
		return e.irb(sb).Call(rtInt, v, vi32(info.Nk)).Text
	case TDouble:
		return e.irb(sb).Call(rtFloat, v, vi32(info.Nk)).Text
	case TI1:
		return e.irb(sb).Call(rtBool, v).Text
	case TI32:
		return e.irb(sb).Call(rtChar, v).Text
	}
	panic("llgen: boxNative: unknown scalar type")
}

// unboxScalar is a borrow, like the plain (non-scalar) e.load it stands in
// for: it never retains.
func (e *emitter) unboxScalar(sb *strings.Builder, ty Type, ptrVal string) Value {
	switch ty {
	case TI64:
		return Value{TI64, e.irb(sb).Call(rtIntVal, vptr(ptrVal)).Text}
	case TDouble:
		return Value{TDouble, e.irb(sb).Call(rtFloatVal, vptr(ptrVal)).Text}
	case TI1:
		return Value{TI1, e.irb(sb).Call(rtTruth, vptr(ptrVal)).Text}
	case TI32:
		return Value{TI32, e.irb(sb).Call(rtCharVal, vptr(ptrVal)).Text}
	}
	panic("llgen: unbox: unknown scalar type")
}

// scalarOperand unboxes l's current box when l isn't itself native (never
// a disqualifying use — see classifyArgs).
func (e *emitter) scalarOperand(sb *strings.Builder, f *mir.Func, l mir.LocalID) (Value, bool) {
	if _, ok := e.scalar[l]; ok {
		return e.loadNative(sb, l), true
	}
	ty, ok := e.scalarType(f.Locals[l].Type)
	if !ok {
		return Value{}, false
	}
	return e.unboxScalar(sb, ty, e.load(sb, l)), true
}

// storeMaybeScalar is the fallback half of classifyDst's contract: dst can
// be classified native even when tryScalarInst declined it (e.g. a `&T`
// operand pre-monomorphization), so the boxed result is unboxed here instead.
func (e *emitter) storeMaybeScalar(sb *strings.Builder, dst mir.LocalID, res string) {
	if info, ok := e.scalar[dst]; ok {
		v := e.unboxScalar(sb, info.Ty, res)
		e.irb(sb).Call(rtRelease, vptr(res))
		e.storeNative(sb, dst, v)
		return
	}
	e.store(sb, dst, res)
}

// storeScalarResult boxes v on the way in if dst wasn't itself classified
// native (some other use of dst disqualified it).
func (e *emitter) storeScalarResult(sb *strings.Builder, f *mir.Func, dst mir.LocalID, v Value) {
	if _, ok := e.scalar[dst]; ok {
		e.storeNative(sb, dst, v)
		return
	}
	e.store(sb, dst, e.boxNative(sb, scalarInfo{Ty: v.Type, Nk: e.numKind(f.Locals[dst].Type)}, v))
}

// scalarBoxOnDemand's caller must release the box right after use; nothing
// else ever will.
func (e *emitter) scalarBoxOnDemand(sb *strings.Builder, l mir.LocalID) string {
	info := e.scalar[l]
	return e.boxNative(sb, info, e.loadNative(sb, l))
}

// releaseScalarArgs releases the ephemeral boxes classifyArgs allows for a
// scalar-classified argument; nothing else will ever release these.
func (e *emitter) releaseScalarArgs(sb *strings.Builder, in mir.Inst, args []string) {
	for i, a := range in.Args {
		if e.isScalar(a) {
			e.irb(sb).Call(rtRelease, vptr(args[i]))
		}
	}
}

// zeroOf initializes a scalar local's alloca the way every other local
// starts out null.
func zeroOf(t Type) string {
	if t == TDouble {
		return "0.0"
	}
	return "0"
}
