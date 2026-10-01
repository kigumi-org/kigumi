package llgen

import (
	"fmt"
	"strings"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

func (e *emitter) sym(f *mir.Func) string {
	// Every cmd/ script has an implicit main; only the entry's is the
	// program's main symbol.
	if f.Name == "main" {
		if f == e.p.Entry {
			return "@\"kigumi.main\""
		}
		return "@\"kigumi.main." + e.entryFile(f.Ent) + "\""
	}
	return "@\"" + e.symName(f) + "\""
}

// entryFile keeps an implicit main's symbol stable across cmd/ scripts.
func (e *emitter) entryFile(ent sem.EntityID) string {
	return e.r.Tree(e.r.Entity(ent).File).File.Name
}

// symName tells overloads apart by position in the overload set, not
// entity number, so symbols don't move when unrelated code changes.
func (e *emitter) symName(f *mir.Func) string {
	if e.r.Entity(f.Ent).Kind == sem.EntImplicitMain {
		return "main." + e.entryFile(f.Ent)
	}
	if e.r.Entity(f.Ent).Kind == sem.EntFn {
		// The bare name is the C symbol of an exported function's wrapper.
		if e.r.Fn(f.Ent).Export != "" {
			return f.Name + "$kg"
		}
		if set := e.r.Fn(f.Ent).Set; set != 0 && len(e.r.Overloads[set].Members) > 1 {
			for i, m := range e.r.Overloads[set].Members {
				if m == f.Ent {
					return fmt.Sprintf("%s#%d", f.Name, i)
				}
			}
		}
	}
	return f.Name
}

func (e *emitter) adapterSym(f *mir.Func) string { return "@\"" + e.symName(f) + "$adapter\"" }

// stdAdapterSym lets a bodiless std function be reached through a
// function-pointer slot (a resource drop, a method table) like a Kigumi one.
func (e *emitter) stdAdapterSym(fn sem.EntityID) string {
	return "@\"" + e.stdKey(fn) + "$stdadapter\""
}

func (e *emitter) stdAdapter(fn sem.EntityID) string {
	return e.stdAdapterWith(fn, e.stdAdapterSym(fn), 0)
}

// stdAdapterWith reads the first bound values from the environment (a
// method value's receiver) and the rest from the call's arguments.
func (e *emitter) stdAdapterWith(fn sem.EntityID, sym string, bound int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "define ptr %s(ptr %%env, ptr %%args) {\n", sym)
	n := len(e.r.Types.Node(e.r.Fn(fn).Sig).Args)
	if e.r.Fn(fn).Recv != sem.RecvNone {
		n++
	}
	var vals []string
	for i := range n {
		v := fmt.Sprintf("%%a%d", i)
		if i < bound {
			e.irb(&sb).CallAs(v, rtAt, vptr("%env"), vi64(i))
		} else {
			e.irb(&sb).CallAs(v, rtAt, vptr("%args"), vi64(i-bound))
		}
		vals = append(vals, v)
	}
	// A primitive llgen expands at call sites (a CValue's drop) is
	// expanded here too, so the runtime never sees its key.
	sig := e.r.Types.Node(e.r.Fn(fn).Sig)
	var types []sem.TypeID
	if e.r.Fn(fn).Recv != sem.RecvNone {
		types = append(types, 0)
	}
	types = append(types, sig.Args...)
	if res, ok := e.ffiRecordCall(&sb, e.stdKey(fn), vals, types, sig.Elem); ok {
		fmt.Fprintf(&sb, "  ret ptr %s\n}\n\n", res)
		return sb.String()
	}
	if info := e.r.Fn(fn); info.Owner != 0 && len(vals) == 1 {
		if res, ok := e.convertCall(&sb, fn, e.stdKey(fn), vals, e.r.Entity(info.Owner).Type, sig.Elem); ok {
			fmt.Fprintf(&sb, "  ret ptr %s\n}\n\n", res)
			return sb.String()
		}
	}
	arr := e.argArray(&sb, vals)
	if op, ok := stdOpFor(e.stdKey(fn)); ok {
		e.irb(&sb).CallAs("%r", rtStdId, vi32(op), vi32(n), vptr(arr))
	} else {
		e.irb(&sb).CallAs("%r", rtStd, vptr(e.cstr(e.stdKey(fn))), vi32(n), vptr(arr))
	}
	sb.WriteString("  ret ptr %r\n}\n\n")
	return sb.String()
}

// adapter wraps a function as a closure body taking an environment array
// and an argument array, for function values and dynamic dispatch.
func (e *emitter) adapter(f *mir.Func) string {
	var sb strings.Builder
	ncaps := 0
	if e.r.Entity(f.Ent).Kind == sem.EntClosure {
		ncaps = len(e.r.Closure(f.Ent).Captures)
	}
	fmt.Fprintf(&sb, "define ptr %s(ptr %%env, ptr %%args) {\n", e.adapterSym(f))
	var vals []string
	for i := range f.Params {
		v := fmt.Sprintf("%%a%d", i)
		if i < ncaps {
			e.irb(&sb).CallAs(v, rtAt, vptr("%env"), vi64(i))
		} else {
			e.irb(&sb).CallAs(v, rtAt, vptr("%args"), vi64(i-ncaps))
		}
		vals = append(vals, "ptr "+v)
	}
	fmt.Fprintf(&sb, "  %%r = call ptr %s(%s)\n  ret ptr %%r\n}\n\n", e.sym(f), strings.Join(vals, ", "))
	return sb.String()
}

// methodAdapterSym is for a method value (a captured receiver), unlike
// adapterSym's plain form, which reads every parameter from the arguments.
func (e *emitter) methodAdapterSym(f *mir.Func) string {
	return "@\"" + e.symName(f) + "$methodvalue\""
}

// methodAdapter reads the receiver from the environment and the rest of
// the parameters from the arguments; adapterSym's plain form assumes no
// capture, which doesn't fit this case.
func (e *emitter) methodAdapter(f *mir.Func) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "define ptr %s(ptr %%env, ptr %%args) {\n", e.methodAdapterSym(f))
	var vals []string
	for i := range f.Params {
		v := fmt.Sprintf("%%a%d", i)
		if i == 0 {
			e.irb(&sb).CallAs(v, rtAt, vptr("%env"), vi64(0))
		} else {
			e.irb(&sb).CallAs(v, rtAt, vptr("%args"), vi64(i-1))
		}
		vals = append(vals, "ptr "+v)
	}
	fmt.Fprintf(&sb, "  %%r = call ptr %s(%s)\n  ret ptr %%r\n}\n\n", e.sym(f), strings.Join(vals, ", "))
	return sb.String()
}

// bindSym names the adapter of a function whose trailing witness
// parameters come from a closure environment (mir.OpBind).
func (e *emitter) bindSym(f *mir.Func) string { return "@\"" + e.symName(f) + "$bind\"" }

// bindAdapter: rt_at's retain on the witness slots offsets invoke's
// release of the argument array, so a lent receiver's refcount stays
// balanced and the caller's reference is untouched.
func (e *emitter) bindAdapter(f *mir.Func) string {
	var sb strings.Builder
	info := e.r.Fn(f.Ent)
	nw := len(info.Witnesses) + len(info.ConstParams)
	fmt.Fprintf(&sb, "define ptr %s(ptr %%env, ptr %%args) {\n", e.bindSym(f))
	var vals []string
	plain := len(f.Params) - nw
	for i := range f.Params {
		v := fmt.Sprintf("%%a%d", i)
		if i < plain {
			e.irb(&sb).CallAs(v, rtAt, vptr("%args"), vi64(i))
		} else {
			e.irb(&sb).CallAs(v, rtAt, vptr("%env"), vi64(i-plain))
		}
		vals = append(vals, "ptr "+v)
	}
	fmt.Fprintf(&sb, "  %%r = call ptr %s(%s)\n  ret ptr %%r\n}\n\n", e.sym(f), strings.Join(vals, ", "))
	return sb.String()
}
