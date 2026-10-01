package llgen

import (
	"fmt"
	"strings"

	"kigumi/internal/sem"
)

// cOffsets lays fields out the way the C compiler does: natural alignment
// unless packed, in which case every field is align 1.
func (e *emitter) cOffsets(ent sem.EntityID) (offs, aligns []int64) {
	info := e.r.TypeDecl(ent)
	packed := info.Layout == "packed"
	var off int64
	for _, f := range info.Fields {
		size, align := e.cLayoutOf(e.r.Entity(f).Type)
		if packed {
			align = 1
		}
		off = (off + align - 1) / align * align
		offs = append(offs, off)
		aligns = append(aligns, align)
		off += size
	}
	return offs, aligns
}

// cReader builds @kg_cread_<type>(ptr)->ptr once per type. cap bounds the
// alignment guaranteed for %p (0 = the type's natural alignment); a record
// nested in a layout(packed) field must inherit it so it doesn't over-claim.
func (e *emitter) cReader(ent sem.EntityID, cap int64) string {
	name := "@kg_cread_" + e.descSym(ent) + capSuffix(cap)
	if e.cio[name] {
		return name
	}
	e.cio[name] = true
	info := e.r.TypeDecl(ent)
	offs, aligns := e.cOffsets(ent)
	capAligns(aligns, cap)
	var sb strings.Builder
	fmt.Fprintf(&sb, "define ptr %s(ptr %%p) {\nentry:\n", name)
	n := len(info.Fields)
	fmt.Fprintf(&sb, "  %%fields = alloca [%d x ptr]\n", max(n, 1))
	for i, f := range info.Fields {
		t := e.r.Entity(f).Type
		at := fmt.Sprintf("%%at%d", i)
		fmt.Fprintf(&sb, "  %s = getelementptr i8, ptr %%p, i64 %d\n", at, offs[i])
		var v string
		if elem, fixedN, ok := e.fixedArrayShape(t); ok {
			v = e.readFixedArray(&sb, at, t, elem, fixedN, aligns[i])
		} else if e.isCRecord(t) {
			v = e.call(&sb, strings.TrimPrefix(e.cReader(e.r.Types.Node(t).Ent, aligns[i]), "@"), []string{"ptr " + at})
		} else {
			ct := e.cType(t)
			raw := e.newTmp()
			fmt.Fprintf(&sb, "  %s = load %s, ptr %s, align %d\n", raw, ct, at, aligns[i])
			v = e.box(&sb, raw, t, ct)
		}
		fmt.Fprintf(&sb, "  %%slot%d = getelementptr [%d x ptr], ptr %%fields, i64 0, i64 %d\n  store ptr %s, ptr %%slot%d\n", i, max(n, 1), i, v, i)
	}
	r := e.irb(&sb).Call(rtRecord, vptr(e.desc(ent)), vi64(n), vptr("%fields")).Text
	fmt.Fprintf(&sb, "  ret ptr %s\n}\n\n", r)
	e.helpers.WriteString(sb.String())
	return name
}

// cWriter builds @kg_cwrite_<type>(ptr, ptr) once per type; cap is as in cReader.
func (e *emitter) cWriter(ent sem.EntityID, cap int64) string {
	name := "@kg_cwrite_" + e.descSym(ent) + capSuffix(cap)
	if e.cio[name] {
		return name
	}
	e.cio[name] = true
	info := e.r.TypeDecl(ent)
	offs, aligns := e.cOffsets(ent)
	capAligns(aligns, cap)
	var sb strings.Builder
	fmt.Fprintf(&sb, "define void %s(ptr %%p, ptr %%rec) {\nentry:\n", name)
	for i, f := range info.Fields {
		t := e.r.Entity(f).Type
		at := fmt.Sprintf("%%at%d", i)
		fmt.Fprintf(&sb, "  %s = getelementptr i8, ptr %%p, i64 %d\n", at, offs[i])
		fv := e.irb(&sb).Call(rtField, vptr("%rec"), vi64(i)).Text
		if elem, fixedN, ok := e.fixedArrayShape(t); ok {
			e.writeFixedArray(&sb, at, fv, elem, fixedN, aligns[i])
		} else if e.isCRecord(t) {
			fmt.Fprintf(&sb, "  call void %s(ptr %s, ptr %s)\n", e.cWriter(e.r.Types.Node(t).Ent, aligns[i]), at, fv)
		} else {
			ct := e.cType(t)
			raw := e.unbox(&sb, fv, t, ct)
			fmt.Fprintf(&sb, "  store %s %s, ptr %s, align %d\n", ct, raw, at, aligns[i])
		}
		e.irb(&sb).Call(rtRelease, vptr(fv))
	}
	sb.WriteString("  ret void\n}\n\n")
	e.helpers.WriteString(sb.String())
	return name
}

// ffiRecordCall reports ok=false for any key it does not handle.
func (e *emitter) ffiRecordCall(sb *strings.Builder, key string, args []string, types []sem.TypeID, dst sem.TypeID) (string, bool) {
	tt := e.r.Types
	elem := func(t sem.TypeID) sem.TypeID {
		if n := tt.Node(t); n.Kind == sem.KRef || n.Kind == sem.KPtr {
			return n.Elem
		}
		return t
	}
	recOf := func(t sem.TypeID) sem.EntityID { return tt.Node(elem(t)).Ent }
	handle := func(cv string) string {
		h := e.irb(sb).Call(rtField, vptr(cv), vi64(0)).Text
		raw := e.irb(sb).Call(rtIntVal, vptr(h)).Text
		e.irb(sb).Call(rtRelease, vptr(h))
		p := e.newTmp()
		fmt.Fprintf(sb, "  %s = inttoptr i64 %s to ptr\n", p, raw)
		return p
	}
	rawPtr := func(v string) string {
		return e.irb(sb).Call(rtPtrVal, vptr(v)).Text
	}
	switch key {
	case "std/ffi.sizeOf":
		size, _ := e.cLayoutOf(elem(types[0]))
		return e.irb(sb).Call(rtInt, vi64(int(size)), vi32(e.numKind(sem.TyUsize))).Text, true
	case "std/ffi.alignOf":
		_, align := e.cLayoutOf(elem(types[0]))
		return e.irb(sb).Call(rtInt, vi64(int(align)), vi32(e.numKind(sem.TyUsize))).Text, true
	case "std/ffi.readRecord":
		return e.call(sb, strings.TrimPrefix(e.cReader(recOf(dst), 0), "@"), []string{"ptr " + rawPtr(args[0])}), true
	case "std/ffi.writeRecord":
		fmt.Fprintf(sb, "  call void %s(ptr %s, ptr %s)\n", e.cWriter(recOf(types[1]), 0), rawPtr(args[0]), args[1])
		return e.irb(sb).Call(rtUnit).Text, true
	case "std/ffi.CValue.new":
		t := types[0]
		size, _ := e.cLayoutOf(t)
		ct := e.cType(sem.TyUsize)
		e.foreign["calloc"] = fmt.Sprintf("declare ptr @calloc(%s, %s)", ct, ct)
		mem := e.newTmp()
		fmt.Fprintf(sb, "  %s = call ptr @calloc(%s 1, %s %d)\n", mem, ct, ct, max(size, 1))
		if e.isCRecord(t) {
			fmt.Fprintf(sb, "  call void %s(ptr %s, ptr %s)\n", e.cWriter(recOf(t), 0), mem, args[0])
		} else {
			e.storeScalar(sb, args[0], t, mem)
		}
		asInt := e.newTmp()
		fmt.Fprintf(sb, "  %s = ptrtoint ptr %s to i64\n", asInt, mem)
		h := e.irb(sb).Call(rtInt, vi64text(asInt), vi32(e.numKind(sem.TyI64))).Text
		fields := e.argArray(sb, []string{h})
		return e.irb(sb).Call(rtRecord, vptr(e.descOfType(dst)), vi64(1), vptr(fields)).Text, true
	case "std/ffi.CValue.ptr":
		return e.irb(sb).Call(rtPtr, vptr(handle(args[0]))).Text, true
	case "std/ffi.CValue.get":
		t := dst
		if e.isCRecord(t) {
			return e.call(sb, strings.TrimPrefix(e.cReader(recOf(t), 0), "@"), []string{"ptr " + handle(args[0])}), true
		}
		return e.loadScalar(sb, t, handle(args[0])), true
	case "std/ffi.CValue.set":
		t := elem(types[1])
		if e.isCRecord(t) {
			fmt.Fprintf(sb, "  call void %s(ptr %s, ptr %s)\n", e.cWriter(recOf(t), 0), handle(args[0]), args[1])
		} else {
			e.storeScalar(sb, args[1], t, handle(args[0]))
		}
		return e.irb(sb).Call(rtUnit).Text, true
	case "std/ffi.CValue.drop":
		e.foreign["free"] = "declare void @free(ptr)"
		fmt.Fprintf(sb, "  call void @free(ptr %s)\n", handle(args[0]))
		return e.irb(sb).Call(rtUnit).Text, true
	}
	return e.ffiScalarCall(sb, key, args, types, dst)
}

// errorAsCall passes two descriptors because a generic E can't check
// `e is E(x)` directly at runtime (E101): E itself, and
// Context for rt_error_as's wrap-chain walk.
func (e *emitter) errorAsCall(sb *strings.Builder, arg string, dst sem.TypeID) string {
	opt, _ := e.r.Types.IsOption(dst)
	want := e.r.Types.Node(opt).Ent
	ctxEnt := e.r.LangItem("Context")
	ctxDesc, wrapped := "null", 0
	if ctxEnt != 0 {
		ctxDesc = e.desc(ctxEnt)
		wrapped = e.r.Field(e.r.FindField(ctxEnt, "wrapped")).Index
	}
	return e.irb(sb).Call(rtErrorAs, vptr(arg), vptr(e.desc(want)), vptr(ctxDesc), vi64(wrapped)).Text
}
