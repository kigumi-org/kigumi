package llgen

import (
	"fmt"
	"strings"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

func (e *emitter) isCRecord(t sem.TypeID) bool {
	n := e.r.Types.Node(t)
	return n.Kind == sem.KNamed && e.r.Entity(n.Ent).File != 0 && e.r.TypeDecl(n.Ent).Layout != ""
}

// byValue calls go through the C shim because struct classification is
// the C compiler's job, not ours.
func (e *emitter) byValue(sig sem.TypeID) bool {
	sn := e.r.Types.Node(sig)
	if e.isCRecord(sn.Elem) {
		return true
	}
	for _, a := range sn.Args {
		if e.isCRecord(a) {
			return true
		}
	}
	return false
}

// cLayoutOf uses natural alignment unless packed (widened by an explicit
// align); pointers are as wide as the target says.
func (e *emitter) cLayoutOf(t sem.TypeID) (size, align int64) {
	tt := e.r.Types
	switch {
	case tt.IsNumeric(t):
		b := int64(tt.Width(t) / 8)
		return b, b
	case t == sem.TyBool:
		return 1, 1
	case t == sem.TyChar:
		return 4, 4
	case tt.Kind(t) == sem.KPtr || tt.IsCFn(t):
		p := int64(tt.PtrBits() / 8)
		return p, p
	}
	if elem, n, ok := e.fixedArrayShape(t); ok {
		es, ea := e.cLayoutOf(elem)
		return es * n, ea
	}
	info := e.r.TypeDecl(tt.Node(t).Ent)
	packed := info.Layout == "packed"
	var off, maxAlign int64 = 0, 1
	for _, f := range info.Fields {
		fs, fa := e.cLayoutOf(e.r.Entity(f).Type)
		if packed {
			fa = 1
		}
		off = (off+fa-1)/fa*fa + fs
		maxAlign = max(maxAlign, fa)
	}
	maxAlign = max(maxAlign, info.Align)
	return (off + maxAlign - 1) / maxAlign * maxAlign, maxAlign
}

// shimCall: extra are arguments the shim takes before the record pointers
// (a function pointer).
func (e *emitter) shimCall(sb *strings.Builder, shim string, sig sem.TypeID, extra, args []string) string {
	sn := e.r.Types.Node(sig)
	vals := append([]string{}, extra...)
	out := ""
	if e.isCRecord(sn.Elem) {
		size, _ := e.cLayoutOf(sn.Elem)
		out = e.stackSlot(sb, fmt.Sprintf("i8, i64 %d, align 16", size))
		vals = append(vals, "ptr "+out)
	}
	for i, pt := range sn.Args {
		if e.isCRecord(pt) {
			size, _ := e.cLayoutOf(pt)
			b := e.stackSlot(sb, fmt.Sprintf("i8, i64 %d, align 16", size))
			fmt.Fprintf(sb, "  call void %s(ptr %s, ptr %s)\n", e.cWriter(e.r.Types.Node(pt).Ent, 0), b, args[i])
			vals = append(vals, "ptr "+b)
			continue
		}
		ct := e.cType(pt)
		vals = append(vals, ct+" "+e.unbox(sb, args[i], pt, ct))
	}
	if out != "" {
		fmt.Fprintf(sb, "  call void @%s(%s)\n", shim, strings.Join(vals, ", "))
		return e.call(sb, strings.TrimPrefix(e.cReader(e.r.Types.Node(sn.Elem).Ent, 0), "@"), []string{"ptr " + out})
	}
	ret := e.cType(sn.Elem)
	if ret == "void" {
		fmt.Fprintf(sb, "  call void @%s(%s)\n", shim, strings.Join(vals, ", "))
		return e.call(sb, "rt_unit", nil)
	}
	raw := e.newTmp()
	fmt.Fprintf(sb, "  %s = call %s @%s(%s)\n", raw, ret, shim, strings.Join(vals, ", "))
	return e.box(sb, raw, sn.Elem, ret)
}

// exportByval defines kg_export_<name>, the LLVM side of the C symbol the
// shim defines for an export(C) function whose records move by value.
func (e *emitter) exportByval(f *mir.Func) string {
	var sb strings.Builder
	sn := e.r.Types.Node(e.r.Fn(f.Ent).Sig)
	name := e.r.Entity(f.Ent).Name
	outRet := e.isCRecord(sn.Elem)
	var params, vals []string
	if outRet {
		params = append(params, "ptr %out")
	}
	for i, pt := range sn.Args {
		ct := "ptr"
		if !e.isCRecord(pt) {
			ct = e.cType(pt)
		}
		params = append(params, fmt.Sprintf("%s %%a%d", ct, i))
	}
	ret := "void"
	if !outRet {
		ret = e.cType(sn.Elem)
	}
	fmt.Fprintf(&sb, "define %s @kg_export_%s(%s) {\nentry:\n", ret, name, strings.Join(params, ", "))
	for i, pt := range sn.Args {
		a := fmt.Sprintf("%%a%d", i)
		switch {
		case deadParam(f, i):
			// See exportWrapper's identical check: a param the body only
			// drops needs no real box, record-shaped or not.
			vals = append(vals, "ptr "+e.irb(&sb).Call(rtUnit).Text)
		case e.isCRecord(pt):
			vals = append(vals, "ptr "+e.call(&sb, strings.TrimPrefix(e.cReader(e.r.Types.Node(pt).Ent, 0), "@"), []string{"ptr " + a}))
		default:
			vals = append(vals, "ptr "+e.box(&sb, a, pt, e.cType(pt)))
		}
	}
	r := e.newTmp()
	fmt.Fprintf(&sb, "  %s = call ptr %s(%s)\n", r, e.sym(f), strings.Join(vals, ", "))
	switch {
	case outRet:
		fmt.Fprintf(&sb, "  call void %s(ptr %%out, ptr %s)\n", e.cWriter(e.r.Types.Node(sn.Elem).Ent, 0), r)
		fmt.Fprintf(&sb, "  call void @rt_release(ptr %s)\n  ret void\n}\n\n", r)
	case ret == "void":
		fmt.Fprintf(&sb, "  call void @rt_release(ptr %s)\n  ret void\n}\n\n", r)
	default:
		out := e.unbox(&sb, r, sn.Elem, ret)
		fmt.Fprintf(&sb, "  call void @rt_release(ptr %s)\n  ret %s %s\n}\n\n", r, ret, out)
	}
	e.exportShim(f)
	return sb.String()
}
