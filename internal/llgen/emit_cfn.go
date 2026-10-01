package llgen

import (
	"fmt"
	"strings"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

func (e *emitter) cCall(sb *strings.Builder, callee string, sig sem.TypeID, args []string, types []sem.TypeID) string {
	sn := e.r.Types.Node(sig)
	var params, vals []string
	for i, pt := range sn.Args {
		if i >= len(args) {
			break
		}
		ct := e.cType(pt)
		params = append(params, ct)
		vals = append(vals, ct+" "+e.unbox(sb, args[i], pt, ct))
	}
	if sn.Flags&sem.FnCVariadic != 0 {
		for i := len(sn.Args); i < len(args); i++ {
			vals = append(vals, e.cArg(sb, args[i], types[i]))
		}
		params = append(params, "...")
	}
	ret := e.cType(sn.Elem)
	fnType := fmt.Sprintf("%s (%s)", ret, strings.Join(params, ", "))
	if ret == "void" {
		fmt.Fprintf(sb, "  call %s %s(%s)\n", fnType, callee, strings.Join(vals, ", "))
		return e.irb(sb).Call(rtUnit).Text
	}
	raw := e.newTmp()
	fmt.Fprintf(sb, "  %s = call %s %s(%s)\n", raw, fnType, callee, strings.Join(vals, ", "))
	return e.box(sb, raw, sn.Elem, ret)
}

func (e *emitter) declareForeign(ent sem.EntityID) string {
	name := e.r.Entity(ent).Name
	sn := e.r.Types.Node(e.r.Fn(ent).Sig)
	var params []string
	for _, pt := range sn.Args {
		params = append(params, e.cType(pt))
	}
	if sn.Flags&sem.FnCVariadic != 0 {
		params = append(params, "...")
	}
	e.foreign[name] = fmt.Sprintf("declare %s @%s(%s)", e.cType(sn.Elem), name, strings.Join(params, ", "))
	return "@" + name
}

// cFnPtr declares the callee when it is a shim-defined by-value wrapper,
// since the shim itself only emits the export(C) side.
func (e *emitter) cFnPtr(sb *strings.Builder, ent sem.EntityID) string {
	return e.irb(sb).Call(rtPtr, vptr(e.cSymbol(ent))).Text
}

func (e *emitter) cSymbol(ent sem.EntityID) string {
	name := e.r.Entity(ent).Name
	switch {
	case e.r.Fn(ent).Abi != "", e.r.Fn(ent).Naked:
		return e.declareForeign(ent)
	case e.byValue(e.r.Fn(ent).Sig):
		e.foreign[name] = "declare void @" + name + "()"
	}
	return "@" + name
}

func (e *emitter) cPtrCall(sb *strings.Builder, fv string, sig sem.TypeID, args []string, types []sem.TypeID) string {
	p := e.irb(sb).Call(rtPtrVal, vptr(fv)).Text
	if e.byValue(sig) {
		return e.shimCall(sb, e.callShim(sig), sig, []string{"ptr " + p}, args)
	}
	return e.cCall(sb, p, sig, args, types)
}

func (e *emitter) dlCall(sb *strings.Builder, f *mir.Func, in mir.Inst, args []string) string {
	recv := e.r.Types.Node(f.Locals[in.Args[0]].Type)
	if recv.Kind == sem.KRef {
		recv = e.r.Types.Node(recv.Elem)
	}
	var types []sem.TypeID
	for _, a := range in.Args[1:] {
		types = append(types, f.Locals[a].Type)
	}
	fp := e.irb(sb).Call(rtField, vptr(args[0]), vi64(1)).Text
	res := e.cPtrCall(sb, fp, recv.Args[0], args[1:], types)
	e.irb(sb).Call(rtRelease, vptr(fp))
	return res
}
