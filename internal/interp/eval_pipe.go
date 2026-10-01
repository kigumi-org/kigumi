package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// fallback evaluates `a || b`.
func (fr *frame) fallback(n, lhs, rhs syntax.NodeID) (Value, *ctrl) {
	l, c := fr.expr(lhs)
	if c != nil {
		return nil, c
	}
	hit, miss, ok := fr.branch(n, deref(l))
	if ok {
		return fr.coerce(n, hit), nil
	}
	if fr.t.Kind(rhs) == syntax.Lambda {
		ln := fr.t.Nodes[rhs]
		params := fr.t.Children(syntax.NodeID(ln.Lhs))
		fr.declare(fr.info.Defs[params[0]], miss)
		return fr.expr(syntax.NodeID(ln.Rhs))
	}
	return fr.expr(rhs)
}

// branch splits a Fallback value into hit or miss.
func (fr *frame) branch(n syntax.NodeID, v Value) (hit, miss Value, ok bool) {
	switch x := v.(type) {
	case Bool:
		return x, Unit{}, bool(x)
	case *Variant:
		e := fr.in.r.Entity(x.V)
		switch e.Name {
		case "Some", "Ok":
			return x.Payload[0], nil, true
		case "None":
			return nil, Unit{}, false
		case "Err":
			return nil, x.Payload[0], false
		}
		if e.Parent == fr.in.r.LangItem("Branch") {
			if e.Name == "Hit" {
				return x.Payload[0], nil, true
			}
			return nil, x.Payload[0], false
		}
	}
	call := fr.info.Calls[n]
	if call.Callee != 0 {
		out, c := fr.in.callFnValue(fr, call.Callee, n, nil, v)
		if c == nil {
			return fr.branch(n, deref(out))
		}
	}
	fr.panicAt(n, "value is not a Fallback")
	return nil, nil, false
}

// pipe evaluates `a |> b`: a call on the right takes a as its first
// argument (its CallInfo says so), a lambda is applied to a.
func (fr *frame) pipe(n, lhs, rhs syntax.NodeID) (Value, *ctrl) {
	if fr.t.Kind(rhs) != syntax.Lambda {
		return fr.expr(rhs)
	}
	a, c := fr.expr(lhs)
	if c != nil {
		return nil, c
	}
	a = fr.transfer(lhs, a)
	b, c := fr.expr(rhs)
	if c != nil {
		return nil, c
	}
	return fr.in.callValue(fr, deref(b), n, []Value{a})
}

func (fr *frame) binary(n syntax.NodeID) (Value, *ctrl) {
	node := fr.t.Nodes[n]
	lhs, rhs := syntax.NodeID(node.Lhs), syntax.NodeID(node.Rhs)
	if lit, ok := fr.in.r.LiteralOf(fr.t, n); ok && (lit.Kind == sem.LitInt || lit.Kind == sem.LitFloat) {
		return fr.literal(n), nil
	}
	call := fr.info.Calls[n]
	switch call.Kind {
	case sem.CallFallback:
		return fr.fallback(n, lhs, rhs)
	case sem.CallPipe:
		return fr.pipe(n, lhs, rhs)
	}
	if fr.t.Toks[node.Tok].Kind == token.PipeGt {
		// `a |> f`: the name on the right called with a.
		return fr.callWith(n, rhs, nil)
	}
	if call.OpText == "&&" || fr.t.TokText(node.Tok) == "&&" {
		l, c := fr.expr(lhs)
		if c != nil || !deref(l).(Bool) {
			return Bool(false), c
		}
		return fr.expr(rhs)
	}
	l, c := fr.expr(lhs)
	if c != nil {
		return nil, c
	}
	r, c := fr.expr(rhs)
	if c != nil {
		return nil, c
	}
	if call.Kind == sem.CallOperator {
		return fr.callUser(call.Callee, n, []Value{l, r}, nil)
	}
	if call.Kind == sem.CallMethod {
		return fr.protoCompare(n, call, l, r)
	}
	return fr.binaryOp(n, call.Op, deref(l), deref(r)), nil
}

// protoCompare runs a comparison the checker routed to `equals` or
// `compareTo`: a witness value for a type parameter, the type's method
// otherwise; the right operand is lent.
func (fr *frame) protoCompare(n syntax.NodeID, call sem.CallInfo, l, r Value) (Value, *ctrl) {
	arg := r
	if _, isRef := r.(*Ref); !isRef {
		arg = &Ref{Cell: &Cell{V: r}}
	}
	var res Value
	var c *ctrl
	if call.Witness != 0 {
		res, c = fr.in.callValue(fr, deref(fr.cell(call.Witness).V), n, []Value{l, arg})
	} else {
		res, c = fr.in.callFnValue(fr, call.Callee, n, append(append([]Value{arg}, fr.witnessValues(call.Passes)...), fr.constValues(call.ConstArgs)...), l)
	}
	if c != nil {
		return nil, c
	}
	switch call.OpText {
	case "==":
		return res, nil
	case "!=":
		return Bool(!deref(res).(Bool)), nil
	}
	name := fr.in.r.Entity(deref(res).(*Variant).V).Name
	switch call.OpText {
	case "<":
		return Bool(name == "Less"), nil
	case "<=":
		return Bool(name != "Greater"), nil
	case ">":
		return Bool(name == "Greater"), nil
	}
	return Bool(name != "Less"), nil
}
