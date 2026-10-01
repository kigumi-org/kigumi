package llgen

import (
	"fmt"
	"strings"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

func (e *emitter) cType(t sem.TypeID) string {
	tt := e.r.Types
	switch {
	case t == sem.TyUnit, t == sem.TyNever:
		return "void"
	case tt.IsFloat(t):
		if tt.Width(t) == 32 {
			return "float"
		}
		return "double"
	case tt.IsInteger(t):
		return fmt.Sprintf("i%d", tt.Width(t))
	}
	return "ptr"
}

// cArg applies C's variadic default argument promotions: floats widen to
// double, narrow integers to int.
func (e *emitter) cArg(sb *strings.Builder, v string, t sem.TypeID) string {
	tt := e.r.Types
	switch {
	case tt.IsFloat(t):
		return "double " + e.unbox(sb, v, t, "double")
	case tt.IsInteger(t) && tt.Width(t) > 32:
		return "i64 " + e.unbox(sb, v, t, "i64")
	case tt.IsInteger(t):
		return "i32 " + e.unbox(sb, v, t, "i32")
	}
	return "ptr " + e.unbox(sb, v, t, "ptr")
}

func (e *emitter) foreignCall(sb *strings.Builder, ent sem.EntityID, args []string, types []sem.TypeID) string {
	sig := e.r.Fn(ent).Sig
	if e.byValue(sig) {
		return e.shimCall(sb, e.byvalShim(ent), sig, nil, args)
	}
	return e.cCall(sb, e.declareForeign(ent), sig, args, types)
}

func (e *emitter) exportWrapper(f *mir.Func) string {
	var sb strings.Builder
	tt := e.r.Types
	sig := tt.Node(e.r.Fn(f.Ent).Sig)
	var params, vals []string
	for i, pt := range sig.Args {
		params = append(params, fmt.Sprintf("%s %%a%d", e.cType(pt), i))
	}
	ret := e.cType(sig.Elem)
	fmt.Fprintf(&sb, "define %s @%s(%s) {\nentry:\n", ret, e.r.Entity(f.Ent).Name, strings.Join(params, ", "))
	for i, pt := range sig.Args {
		// rt_unit() is immortal, so it stands in here: a trivial export(C) fn
		// can end up as a signal handler, where malloc/free must not reenter.
		if deadParam(f, i) {
			vals = append(vals, "ptr "+e.irb(&sb).Call(rtUnit).Text)
			continue
		}
		vals = append(vals, "ptr "+e.box(&sb, fmt.Sprintf("%%a%d", i), pt, e.cType(pt)))
	}
	r := e.newTmp()
	fmt.Fprintf(&sb, "  %s = call ptr %s(%s)\n", r, e.sym(f), strings.Join(vals, ", "))
	if ret == "void" {
		e.irb(&sb).Call(rtRelease, vptr(r))
		sb.WriteString("  ret void\n}\n\n")
		return sb.String()
	}
	out := e.unbox(&sb, r, sig.Elem, ret)
	e.irb(&sb).Call(rtRelease, vptr(r))
	fmt.Fprintf(&sb, "  ret %s %s\n}\n\n", ret, out)
	return sb.String()
}

// deadParam reports whether f's body never reads parameter i, treating
// only MIR's own scope-exit drop (build_scope.go) as a non-use; any other
// reference counts as used, conservatively.
func deadParam(f *mir.Func, i int) bool {
	if i >= len(f.Params) {
		return false
	}
	id := f.Params[i]
	for _, b := range f.Blocks {
		for _, in := range b.Insts {
			trailing := in.Op == mir.OpDrop || in.Op == mir.OpRelease
			if in.Dst == id && !trailing {
				return false
			}
			for _, a := range in.Args {
				if a == id && !trailing {
					return false
				}
			}
		}
		for _, a := range b.Term.Args {
			if a == id {
				return false
			}
		}
	}
	return true
}

func (e *emitter) unbox(sb *strings.Builder, v string, t sem.TypeID, ct string) string {
	tt := e.r.Types
	switch {
	case ct == "float" || ct == "double":
		d := e.irb(sb).Call(rtFloatVal, vptr(v)).Text
		if ct == "float" {
			out := e.newTmp()
			fmt.Fprintf(sb, "  %s = fptrunc double %s to float\n", out, d)
			return out
		}
		return d
	case tt.Kind(t) == sem.KPtr || tt.IsCFn(t):
		return e.irb(sb).Call(rtPtrVal, vptr(v)).Text
	case tt.IsInteger(t):
		w := e.irb(sb).Call(rtIntVal, vptr(v)).Text
		if ct != "i64" {
			out := e.newTmp()
			fmt.Fprintf(sb, "  %s = trunc i64 %s to %s\n", out, w, ct)
			return out
		}
		return w
	}
	return e.irb(sb).Call(rtCstr, vptr(v)).Text
}

func (e *emitter) box(sb *strings.Builder, raw string, t sem.TypeID, ct string) string {
	tt := e.r.Types
	switch {
	case ct == "float":
		d := e.newTmp()
		fmt.Fprintf(sb, "  %s = fpext float %s to double\n", d, raw)
		return e.irb(sb).Call(rtFloat, vdouble(d), vi32(e.numKind(t))).Text
	case ct == "double":
		return e.irb(sb).Call(rtFloat, vdouble(raw), vi32(e.numKind(t))).Text
	case tt.Kind(t) == sem.KPtr || tt.IsCFn(t):
		return e.irb(sb).Call(rtPtr, vptr(raw)).Text
	case tt.IsInteger(t):
		w := raw
		if ct != "i64" {
			w = e.newTmp()
			ext := "sext"
			if !tt.IsSigned(t) {
				ext = "zext"
			}
			fmt.Fprintf(sb, "  %s = %s %s %s to i64\n", w, ext, ct, raw)
		}
		return e.irb(sb).Call(rtInt, vi64text(w), vi32(e.numKind(t))).Text
	}
	return e.irb(sb).Call(rtFromCstr, vptr(raw)).Text
}
