package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"sort"
)

func (in *Interp) isDropFn(fn sem.EntityID) bool {
	owner := in.r.Fn(fn).Owner
	return owner != 0 && in.r.Entity(owner).Kind == sem.EntType && in.r.TypeDecl(owner).Drop == fn
}

// dispatch resolves an interface requirement on the receiver's dynamic
// type (existential or generic).
func (in *Interp) dispatch(fr *frame, req sem.EntityID, n syntax.NodeID, args []Value, recv Value) (Value, *ctrl) {
	name := in.r.Entity(req).Name
	target := deref(recv)
	if box, ok := target.(*Box); ok {
		target = box.V
		if op, isOp := target.(*Opaque); isOp && op.Kind == "Error" && name == "message" {
			return Str(op.Data.(string)), nil
		}
		return in.callMember(fr, box.Dyn, name, n, args, target)
	}
	return in.callMember(fr, in.dynType(target), name, n, args, target)
}

func (in *Interp) callMember(fr *frame, t sem.TypeID, name string, n syntax.NodeID, args []Value, recv Value) (Value, *ctrl) {
	set := in.r.MemberSet(t, name)
	if set == 0 {
		fr.panicAt(n, "no method `%s` on `%s`", name, in.r.TypeString(t))
	}
	m := in.r.Overloads[set].Members[0]
	if in.r.Fn(m).Recv == sem.RecvMut {
		recv = &Ref{Cell: &Cell{V: recv}}
	}
	return in.callFnValue(fr, m, n, args, recv)
}

// dynType recovers the type of a runtime value for dispatch.
func (in *Interp) dynType(v Value) sem.TypeID {
	switch x := v.(type) {
	case Int:
		return x.T
	case Float:
		return x.T
	case Bool:
		return sem.TyBool
	case Str:
		return sem.TyString
	case Bytes:
		return sem.TyBytes
	case Char:
		return sem.TyChar
	case *Record:
		return x.Type
	case *Variant:
		return x.Type
	case *Box:
		return x.Dyn
	}
	return sem.TyUnit
}

// HasBuiltin reports whether a bodiless function has a Go implementation.
func (in *Interp) HasBuiltin(fn sem.EntityID) bool {
	_, ok := in.builtins[in.builtinKey(fn)]
	return ok
}

// BuiltinKeys lists the registered implementations, sorted.
func (in *Interp) BuiltinKeys() []string {
	keys := make([]string, 0, len(in.builtins))
	for k := range in.builtins {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// builtinKey names a bodiless function for the runtime table.
func (in *Interp) builtinKey(fn sem.EntityID) string {
	e := in.r.Entity(fn)
	info := in.r.Fn(fn)
	key := in.r.Packages[e.Pkg].Path + "."
	if info.Owner != 0 {
		key += in.r.Entity(info.Owner).Name + "."
	}
	return key + e.Name
}
