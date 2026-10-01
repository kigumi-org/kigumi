package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func (fr *frame) index(n syntax.NodeID) (Value, *ctrl) {
	node := fr.t.Nodes[n]
	base, c := fr.expr(syntax.NodeID(node.Lhs))
	if c != nil {
		return nil, c
	}
	items := fr.t.Children(syntax.NodeID(node.Rhs))
	iv, c := fr.expr(items[0])
	if c != nil {
		return nil, c
	}
	return fr.indexValue(n, deref(base), deref(iv)), nil
}

func (fr *frame) indexValue(n syntax.NodeID, base, idx Value) Value {
	if rng, ok := idx.(*Record); ok {
		lo, hi := rng.Fields[0].(Int).V, rng.Fields[1].(Int).V
		if bool(rng.Fields[2].(Bool)) {
			hi++
		}
		switch b := base.(type) {
		case Str:
			if lo < 0 || hi > int64(len(b)) || lo > hi || !boundary(string(b), int(lo)) || !boundary(string(b), int(hi)) {
				fr.panicAt(n, "slice %d..%d is not on character boundaries", lo, hi)
			}
			return b[lo:hi]
		case Bytes:
			if lo < 0 || hi > int64(len(b)) || lo > hi {
				fr.panicAt(n, "slice %d..%d out of range", lo, hi)
			}
			return Bytes(append([]byte{}, b[lo:hi]...))
		case *Array:
			if lo < 0 || hi > int64(len(b.Elems)) || lo > hi {
				fr.panicAt(n, "slice %d..%d out of range", lo, hi)
			}
			return &Array{Elems: append([]Value{}, b.Elems[lo:hi]...)}
		}
	}
	i := idx.(Int).V
	switch b := base.(type) {
	case Str:
		if i < 0 || i >= int64(len(b)) {
			fr.panicAt(n, "index %d out of range", i)
		}
		return Int{V: int64(b[i]), T: sem.TyU8}
	case Bytes:
		if i < 0 || i >= int64(len(b)) {
			fr.panicAt(n, "index %d out of range", i)
		}
		return Int{V: int64(b[i]), T: sem.TyU8}
	case *Array:
		if i < 0 || i >= int64(len(b.Elems)) {
			fr.panicAt(n, "index %d out of range", i)
		}
		return b.Elems[i]
	}
	fr.panicAt(n, "value is not indexable")
	return nil
}

func boundary(s string, i int) bool {
	return i == len(s) || s[i]&0xC0 != 0x80
}

func (fr *frame) unary(n syntax.NodeID) (Value, *ctrl) {
	node := fr.t.Nodes[n]
	if lit, ok := fr.in.r.LiteralOf(fr.t, n); ok && lit.Kind != sem.LitString {
		return fr.literal(n), nil
	}
	v, c := fr.expr(syntax.NodeID(node.Lhs))
	if c != nil {
		return nil, c
	}
	if call, ok := fr.info.Calls[n]; ok && call.Kind == sem.CallOperator {
		return fr.callUser(call.Callee, n, []Value{v}, nil)
	}
	switch fr.t.Toks[node.Tok].Kind {
	case token.Minus:
		switch x := deref(v).(type) {
		case Int:
			return fr.checkedInt(n, -x.V, x.T, x.V == minOf(fr.in, x.T)), nil
		case Float:
			return Float{V: -x.V, T: x.T}, nil
		}
	case token.Bang:
		return !deref(v).(Bool), nil
	case token.Tilde:
		x := deref(v).(Int)
		return Int{V: truncate(^x.V, x.T, fr.in), T: x.T}, nil
	}
	fr.panicAt(n, "bad unary operator")
	return nil, nil
}
