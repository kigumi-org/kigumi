package llgen

import (
	"fmt"
	"strings"

	"kigumi/internal/sem"
)

// ffiScalarCall reports ok=false for any key it does not handle.
func (e *emitter) ffiScalarCall(sb *strings.Builder, key string, args []string, types []sem.TypeID, dst sem.TypeID) (string, bool) {
	rawPtr := func(v string) string {
		return e.irb(sb).Call(rtPtrVal, vptr(v)).Text
	}
	switch key {
	case "std/ffi.readU8", "std/ffi.readI32", "std/ffi.readI64", "std/ffi.readF64":
		ct := e.cType(dst)
		raw := e.newTmp()
		fmt.Fprintf(sb, "  %s = load %s, ptr %s\n", raw, ct, rawPtr(args[0]))
		return e.box(sb, raw, dst, ct), true
	case "std/ffi.writeU8", "std/ffi.writeI32", "std/ffi.writeI64", "std/ffi.writeF64":
		ct := e.cType(types[1])
		raw := e.unbox(sb, args[1], types[1], ct)
		fmt.Fprintf(sb, "  store %s %s, ptr %s\n", ct, raw, rawPtr(args[0]))
		return e.irb(sb).Call(rtUnit).Text, true
	case "std/ffi.readPtr":
		raw := e.newTmp()
		fmt.Fprintf(sb, "  %s = load ptr, ptr %s\n", raw, rawPtr(args[0]))
		return e.irb(sb).Call(rtPtr, vptr(raw)).Text, true
	case "std/ffi.writePtr":
		fmt.Fprintf(sb, "  store ptr %s, ptr %s\n", rawPtr(args[1]), rawPtr(args[0]))
		return e.irb(sb).Call(rtUnit).Text, true
	case "std/ffi.null":
		return e.irb(sb).Call(rtPtr, vptr("null")).Text, true
	case "std/ffi.isNull":
		c := e.newTmp()
		fmt.Fprintf(sb, "  %s = icmp eq ptr %s, null\n", c, rawPtr(args[0]))
		return e.irb(sb).Call(rtBool, vi1(c)).Text, true
	case "std/ffi.cast", "std/ffi.toConst", "std/ffi.toMut":
		return e.irb(sb).Call(rtPtr, vptr(rawPtr(args[0]))).Text, true
	case "std/ffi.offset":
		n := e.irb(sb).Call(rtIntVal, vptr(args[1])).Text
		p := e.newTmp()
		fmt.Fprintf(sb, "  %s = getelementptr i8, ptr %s, i64 %s\n", p, rawPtr(args[0]), n)
		return e.irb(sb).Call(rtPtr, vptr(p)).Text, true
	}
	return "", false
}

// convertCall passes only the target kind to rt_convert; the source kind
// travels with the value itself.
func (e *emitter) convertCall(sb *strings.Builder, ent sem.EntityID, key string, args []string, self, dst sem.TypeID) (string, bool) {
	tt := e.r.Types
	if !strings.HasPrefix(key, "std/prelude.") || !strings.HasPrefix(e.r.Entity(ent).Name, "to") || e.r.Fn(ent).Owner == 0 {
		return "", false
	}
	if n := tt.Node(self); n.Kind == sem.KRef {
		self = n.Elem
	}
	if !tt.IsNumeric(self) && self != sem.TyChar {
		return "", false
	}
	target, checked := dst, 0
	if n := tt.Node(dst); n.Kind == sem.KNamed && n.Ent == tt.OptionEnt() {
		target, checked = n.Args[0], 1
	}
	dnk := rtConvertCharTarget
	if target != sem.TyChar {
		if !tt.IsNumeric(target) {
			return "", false
		}
		dnk = e.numKind(target)
	}
	return e.irb(sb).Call(rtConvert, vptr(args[0]), vi32(dnk), vi32(checked)).Text, true
}

// loweredStd reports whether a bodiless std function is expanded by the
// compiler rather than rt_std, so its function value needs that same adapter.
func (e *emitter) loweredStd(fn sem.EntityID) bool {
	key := e.stdKey(fn)
	if strings.HasPrefix(key, "std/ffi.") {
		return true
	}
	info := e.r.Fn(fn)
	if !strings.HasPrefix(key, "std/prelude.") || !strings.HasPrefix(e.r.Entity(fn).Name, "to") || info.Owner == 0 {
		return false
	}
	self := e.r.Entity(info.Owner).Type
	return e.r.Types.IsNumeric(self) || self == sem.TyChar
}

func (e *emitter) stdBound(fn sem.EntityID, bound int) string {
	sym := fmt.Sprintf("@\"%s$stdbound%d\"", e.stdKey(fn), bound)
	if !e.cio[sym] {
		e.cio[sym] = true
		e.helpers.WriteString(e.stdAdapterWith(fn, sym, bound))
	}
	return sym
}
