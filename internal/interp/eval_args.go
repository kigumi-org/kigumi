package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// receiver evaluates the receiver of a method: mutable receivers pass the
// place, `move self` moves the value, `self` shares it.
func (fr *frame) receiver(base syntax.NodeID, m sem.EntityID) (Value, *ctrl) {
	info := fr.in.r.Fn(m)
	switch info.Recv {
	case sem.RecvMut:
		if v, c := fr.tryBorrow(base); v != nil || c != nil {
			return v, c
		}
	case sem.RecvMove:
		v, c := fr.expr(base)
		if c != nil {
			return nil, c
		}
		return fr.transfer(base, v), nil
	}
	v, c := fr.expr(base)
	if c != nil {
		return nil, c
	}
	return v, nil
}

// isBorrowExpr reports whether n is a `&`/`&mut` expression (through parens),
// whose evaluated value is already a *Ref that a call argument must keep
// rather than deref-and-copy.
func isBorrowExpr(t *syntax.Tree, n syntax.NodeID) bool {
	for {
		switch t.Kind(n) {
		case syntax.Paren:
			n = syntax.NodeID(t.Nodes[n].Lhs)
		case syntax.BorrowExpr:
			return true
		default:
			return false
		}
	}
}

// isReborrow reports whether n is a bare `&T`/`&mut T` local that must
// reborrow rather than get the deref-and-clone plain
// arguments do: `&mut T` always, a shared `&T` only when T is not Copy. An
// async callee never reborrows, since its future can outlive the call.
func (fr *frame) isReborrow(n syntax.NodeID, idx int, call sem.CallInfo) bool {
	for fr.t.Kind(n) == syntax.Paren {
		n = syntax.NodeID(fr.t.Nodes[n].Lhs)
	}
	if fr.t.Kind(n) != syntax.Ident {
		return false
	}
	switch call.Kind {
	case sem.CallFn, sem.CallAssoc, sem.CallOperator:
		if fr.in.r.Fn(call.Callee).Declared&sem.EffAsync != 0 {
			return false
		}
	}
	t := fr.typeOf(n)
	if fr.in.r.Types.Kind(t) != sem.KRef {
		return false
	}
	mut := !fr.in.r.IsCopy(t)
	elemCopy := fr.in.r.IsCopy(fr.in.r.Types.Node(t).Elem)
	return (mut || !elemCopy) && declaredParamRef(fr.in.r, call, idx)
}

// declaredParamRef checks the callee's idx-th parameter pre-instantiation,
// since a type parameter unified to `&mut U` isn't written as `&mut T` in
// the signature and E220 would miss the escaping reborrow. Other
// call kinds fall back to the old type-only check (mirrors argValue/
// declaredRef in internal/mir/build_call.go).
func declaredParamRef(r *sem.Result, call sem.CallInfo, idx int) bool {
	switch call.Kind {
	case sem.CallMethod, sem.CallStatic, sem.CallFn, sem.CallAssoc, sem.CallOperator:
	default:
		return true
	}
	params := r.Fn(call.Callee).Params
	if idx < 0 || idx >= len(params) {
		return false
	}
	return r.Types.Kind(r.Entity(params[idx]).Type) == sem.KRef
}

func (fr *frame) tryBorrow(n syntax.NodeID) (Value, *ctrl) {
	switch fr.t.Kind(n) {
	case syntax.Ident, syntax.MemberExpr, syntax.BracketExpr, syntax.Paren:
		if fr.t.Kind(n) == syntax.Ident && fr.in.r.Entity(fr.info.Uses[n]).Kind != sem.EntLocal && fr.in.r.Entity(fr.info.Uses[n]).Kind != sem.EntParam {
			return nil, nil
		}
		return fr.borrow(n)
	}
	return nil, nil
}

// args evaluates call arguments; variadic element lists become one array.
// A piped left operand comes first, unless ArgOrder already placed it (a
// named argument at the call site: ArgOrder is then complete).
func (fr *frame) args(nodes []syntax.NodeID, call sem.CallInfo) ([]Value, *ctrl) {
	var out []Value
	if call.Piped != 0 && call.ArgOrder == nil {
		nodes = append([]syntax.NodeID{call.Piped}, nodes...)
	}
	for i, a := range nodes {
		v, c := fr.expr(a)
		if c != nil {
			return nil, c
		}
		if isBorrowExpr(fr.t, a) || fr.isReborrow(a, i, call) {
			out = append(out, v)
			continue
		}
		out = append(out, fr.transfer(a, v))
	}
	if call.Variadic == sem.VariadicElements {
		fixed := len(fr.in.r.Types.Node(fr.in.r.Fn(call.Callee).Sig).Args) - 1
		if fixed < 0 {
			fixed = 0
		}
		if fixed > len(out) {
			fixed = len(out)
		}
		rest := &Array{Elems: append([]Value{}, out[fixed:]...)}
		out = append(out[:fixed], rest)
	}
	return out, nil
}
