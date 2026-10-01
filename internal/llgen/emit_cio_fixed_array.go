package llgen

import (
	"fmt"
	"strings"

	"kigumi/internal/sem"
)

func (e *emitter) descSym(ent sem.EntityID) string {
	x := e.r.Entity(ent)
	return sanitize(e.r.Packages[x.Pkg].Path + "." + x.Name)
}

// fixedArrayShape matches sem's own: the C ABI treats
// FixedArray[elem, N] as inline elem[N], not the boxed Array[elem] it
// otherwise wraps.
func (e *emitter) fixedArrayShape(t sem.TypeID) (elem sem.TypeID, n int64, ok bool) {
	tt := e.r.Types
	if tt.Kind(t) != sem.KNamed {
		return 0, 0, false
	}
	node := tt.Node(t)
	if node.Ent == 0 || node.Ent != e.r.LangItem("FixedArray") || len(node.Args) != 2 || !tt.IsConst(node.Args[1]) {
		return 0, 0, false
	}
	v, _ := tt.ConstValue(node.Args[1])
	return node.Args[0], v, true
}

// readFixedArray handles FixedArray at the C boundary: the runtime shape
// has no inline buffer, so this is the only place that knows the field is
// really elem[n].
func (e *emitter) readFixedArray(sb *strings.Builder, base string, fixedTy, elem sem.TypeID, n, align int64) string {
	es, _ := e.cLayoutOf(elem)
	elemAlign := min(align, es)
	ct := e.cType(elem)
	// A bare alloca, not stackSlot/argArray: this shim has no entry block
	// for stackSlot's hoisting to collect into.
	items := e.newTmp()
	fmt.Fprintf(sb, "  %s = alloca [%d x ptr]\n", items, max(n, 1))
	for i := int64(0); i < n; i++ {
		at := e.newTmp()
		fmt.Fprintf(sb, "  %s = getelementptr i8, ptr %s, i64 %d\n", at, base, i*es)
		raw := e.newTmp()
		fmt.Fprintf(sb, "  %s = load %s, ptr %s, align %d\n", raw, ct, at, elemAlign)
		v := e.box(sb, raw, elem, ct)
		slot := e.newTmp()
		fmt.Fprintf(sb, "  %s = getelementptr [%d x ptr], ptr %s, i64 0, i64 %d\n  store ptr %s, ptr %s\n", slot, max(n, 1), items, i, v, slot)
	}
	arr := e.irb(sb).Call(rtArray, vi64(int(n)), vptr(items)).Text
	fieldSlot := e.newTmp()
	fmt.Fprintf(sb, "  %s = alloca ptr\n  store ptr %s, ptr %s\n", fieldSlot, arr, fieldSlot)
	return e.irb(sb).Call(rtRecord, vptr(e.descOfType(fixedTy)), vi64(1), vptr(fieldSlot)).Text
}

// writeFixedArray is the cWriter counterpart of readFixedArray.
func (e *emitter) writeFixedArray(sb *strings.Builder, base, fv string, elem sem.TypeID, n, align int64) {
	arr := e.irb(sb).Call(rtField, vptr(fv), vi64(0)).Text
	es, _ := e.cLayoutOf(elem)
	elemAlign := min(align, es)
	ct := e.cType(elem)
	for i := int64(0); i < n; i++ {
		idx := e.irb(sb).Call(rtInt, vi64(int(i)), vi32(e.numKind(sem.TyUsize))).Text
		item := e.irb(sb).Call(rtIndex, vptr(arr), vptr(idx)).Text
		raw := e.unbox(sb, item, elem, ct)
		at := e.newTmp()
		fmt.Fprintf(sb, "  %s = getelementptr i8, ptr %s, i64 %d\n", at, base, i*es)
		fmt.Fprintf(sb, "  store %s %s, ptr %s, align %d\n", ct, raw, at, elemAlign)
		e.irb(sb).Call(rtRelease, vptr(item))
	}
	e.irb(sb).Call(rtRelease, vptr(arr))
}
