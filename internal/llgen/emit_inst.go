package llgen

import (
	"fmt"
	"strings"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

func (e *emitter) inst(sb *strings.Builder, f *mir.Func, in mir.Inst) {
	if e.tryScalarInst(sb, f, in) {
		return
	}
	var args []string
	for _, a := range in.Args {
		if e.isScalar(a) {
			// classifyArgs only lets a scalar-classified local reach here at a
			// non-retaining read; released below, in releaseScalarArgs.
			args = append(args, e.scalarBoxOnDemand(sb, a))
			continue
		}
		args = append(args, e.load(sb, a))
	}
	res := ""
	switch in.Op {
	case mir.OpNop:
		return
	case mir.OpConst:
		res = e.constant(sb, in)
	case mir.OpUnit:
		res = e.irb(sb).Call(rtUnit).Text
	case mir.OpAlias:
		res = args[0]
	case mir.OpShare:
		if in.Copy {
			res = e.irb(sb).Call(rtCopy, vptr(args[0])).Text
		} else {
			res = e.irb(sb).Call(rtRetain, vptr(args[0])).Text
		}
	case mir.OpRelease:
		e.irb(sb).Call(rtRelease, vptr(args[0]))
		return
	case mir.OpMove:
		res = args[0]
		// A moved-out binding may still be dropped on a path that did not
		// move it; a null slot makes that release a no-op.
		if f.Locals[in.Args[0]].Ent != 0 {
			e.store(sb, in.Args[0], "null")
		}
	case mir.OpCall:
		res = e.callEntity(sb, f, in, args, f.Locals[in.Dst].Type)
	case mir.OpCallValue:
		arr := e.argArray(sb, args[1:])
		res = e.irb(sb).Call(rtCallv, vptr(args[0]), vi32(len(args)-1), vptr(arr)).Text
	case mir.OpBuiltin:
		arr := e.argArray(sb, args)
		res = e.irb(sb).Call(rtBuiltin, vptr(e.cstr(in.Str)), vi32(len(args)), vptr(arr)).Text
	case mir.OpBinary:
		res = e.irb(sb).Call(rtBinop, vptr(e.cstr(in.Str)), vptr(args[0]), vptr(args[1])).Text
	case mir.OpUnary:
		if in.Str == "cast" {
			res = e.irb(sb).Call(rtCast, vptr(args[0]), vi32(e.numKind(in.Type))).Text
		} else {
			res = e.irb(sb).Call(rtUnop, vptr(e.cstr(in.Str)), vptr(args[0])).Text
		}
	case mir.OpRecord:
		arr := e.argArray(sb, args)
		res = e.irb(sb).Call(rtRecord, vptr(e.desc(in.Ent)), vi64(len(args)), vptr(arr)).Text
	case mir.OpVariant:
		arr := e.argArray(sb, args)
		res = e.irb(sb).Call(rtVariant, vptr(e.vdesc(in.Ent)), vi64(len(args)), vptr(arr)).Text
	case mir.OpField:
		res = e.irb(sb).Call(rtField, vptr(args[0]), vi64(in.Index)).Text
	case mir.OpFieldMove:
		res = e.irb(sb).Call(rtFieldMove, vptr(args[0]), vi64(in.Index)).Text
	case mir.OpSetField:
		e.irb(sb).Call(rtSetfield, vptr(args[0]), vi64(in.Index), vptr(args[1]))
		return
	case mir.OpArray:
		arr := e.argArray(sb, args)
		res = e.irb(sb).Call(rtArray, vi64(len(args)), vptr(arr)).Text
	case mir.OpIndex:
		res = e.irb(sb).Call(rtIndex, vptr(args[0]), vptr(args[1])).Text
	case mir.OpSetIndex:
		e.irb(sb).Call(rtSetindex, vptr(args[0]), vptr(args[1]), vptr(args[2]))
		e.releaseScalarArgs(sb, in, args)
		return
	case mir.OpPayload:
		res = e.irb(sb).Call(rtPayload, vptr(args[0]), vi64(in.Index)).Text
	case mir.OpIsVariant:
		res = e.irb(sb).Call(rtIsvariant, vptr(args[0]), vptr(e.vdesc(in.Ent))).Text
	case mir.OpIsType:
		res = e.irb(sb).Call(rtIstype, vptr(args[0]), vptr(e.desc(in.Ent))).Text
	case mir.OpUnbox:
		res = e.irb(sb).Call(rtUnbox, vptr(args[0])).Text
	case mir.OpBox:
		res = e.irb(sb).Call(rtBox, vptr(args[0]), vptr(e.descOfType(in.Type)), vptr(e.vtable(in.Type, f.Locals[in.Dst].Type, e.r.Entity(f.Ent).Pkg))).Text
	case mir.OpAsm:
		res = e.asm(sb, f, in, args)
	case mir.OpCFnPtr:
		res = e.cFnPtr(sb, in.Ent)
	case mir.OpCallC:
		var types []sem.TypeID
		for _, a := range in.Args[1:] {
			types = append(types, f.Locals[a].Type)
		}
		res = e.cPtrCall(sb, args[0], in.Type, args[1:], types)
	case mir.OpBind:
		arr := e.argArray(sb, args)
		if fn, ok := e.p.ByEnt[in.Ent]; ok {
			res = e.irb(sb).Call(rtClosure, vptr(e.bindSym(fn)), vi32(len(args)), vptr(arr)).Text
		} else {
			res = e.irb(sb).Call(rtStdBind, vptr(e.cstr(e.stdKey(in.Ent))), vi32(mir.LentMask(e.r, in.Ent)), vi32(len(args)), vptr(arr)).Text
		}
	case mir.OpClosure, mir.OpFnItem:
		if in.Op == mir.OpFnItem && (e.r.Fn(in.Ent).Abi != "" || e.r.Fn(in.Ent).Naked) {
			res = e.cFnPtr(sb, in.Ent)
			break
		}
		arr := e.argArray(sb, args)
		ctor := rtClosure
		if in.Async {
			ctor = rtClosureAsync
		}
		if fn, ok := e.p.ByEnt[in.Ent]; ok {
			sym := e.adapterSym(fn)
			// A method value captures the receiver in the environment; the
			// plain adapter expects self in the call's arguments instead.
			if in.Op == mir.OpClosure && in.Str == "method" {
				sym = e.methodAdapterSym(fn)
			}
			res = e.irb(sb).Call(ctor, vptr(sym), vi32(len(args)), vptr(arr)).Text
		} else if e.loweredStd(in.Ent) {
			res = e.irb(sb).Call(ctor, vptr(e.stdBound(in.Ent, len(args))), vi32(len(args)), vptr(arr)).Text
		} else {
			res = e.irb(sb).Call(rtStdClosure, vptr(e.cstr(e.stdKey(in.Ent))), vi32(len(args)), vptr(arr)).Text
		}
	case mir.OpBorrow:
		res = args[0]
	case mir.OpNewCell:
		res = e.irb(sb).Call(rtCell, vptr(args[0])).Text
	case mir.OpCellGet:
		res = e.irb(sb).Call(rtCellget, vptr(args[0])).Text
	case mir.OpCellSet:
		e.irb(sb).Call(rtCellset, vptr(args[0]), vptr(args[1]))
		return
	case mir.OpBoxReplace:
		e.irb(sb).Call(rtBoxReplace, vptr(args[0]), vptr(args[1]))
		return
	case mir.OpInterp:
		res = e.interp(sb, in, args)
	case mir.OpShell:
		res = e.shell(sb, in, args)
	case mir.OpDrop:
		e.irb(sb).Call(rtDrop, vptr(args[0]))
		// A binding assigned on some paths only (ownMaybe) is dropped again
		// on the next scope exit; a null slot makes that release a no-op.
		if f.Locals[in.Args[0]].Ent != 0 {
			e.store(sb, in.Args[0], "null")
		}
		return
	case mir.OpPanic:
		e.irb(sb).Call(rtPanic, vptr(e.cstr(in.Str)))
		return
	default:
		return
	}
	e.releaseScalarArgs(sb, in, args)
	e.storeMaybeScalar(sb, in.Dst, res)
}

func (e *emitter) call(sb *strings.Builder, fn string, args []string) string {
	t := e.newTmp()
	fmt.Fprintf(sb, "  %s = call ptr @%s(%s)\n", t, fn, joinArgs(args))
	return t
}

func (e *emitter) callVoid(sb *strings.Builder, fn string, args []string) {
	fmt.Fprintf(sb, "  call void @%s(%s)\n", fn, joinArgs(args))
}

func joinArgs(args []string) string {
	var parts []string
	for _, a := range args {
		if strings.HasPrefix(a, "%") || strings.HasPrefix(a, "@") || strings.HasPrefix(a, "null") {
			parts = append(parts, "ptr "+a)
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, ", ")
}
