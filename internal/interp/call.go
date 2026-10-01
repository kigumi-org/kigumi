package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// call dispatches a call expression by the checker's classification.
func (fr *frame) call(n syntax.NodeID) (Value, *ctrl) {
	node := fr.t.Nodes[n]
	call := fr.info.Calls[n]
	// A named argument moved sem's checked args out of
	// source order; ArgOrder carries that order back for evaluation.
	argNodes := call.ArgOrder
	if argNodes == nil {
		argNodes = fr.t.Children(syntax.NodeID(node.Rhs))
	}
	return fr.callWith(n, syntax.NodeID(node.Lhs), argNodes)
}

// callWith evaluates a call of callee on argNodes as classified at n; a
// piped left operand comes first (eval_args.go).
func (fr *frame) callWith(n, callee syntax.NodeID, argNodes []syntax.NodeID) (Value, *ctrl) {
	call := fr.info.Calls[n]
	in := fr.in
	switch call.Kind {
	case sem.CallIntrinsic:
		args, c := fr.args(argNodes, call)
		if c != nil {
			return nil, c
		}
		return in.intrinsic(fr, n, in.r.Entity(call.Callee).Name, args)
	case sem.CallVariant:
		args, c := fr.args(argNodes, call)
		if c != nil {
			return nil, c
		}
		return &Variant{Type: fr.typeOf(n), V: call.Callee, Payload: args}, nil
	case sem.CallMethod:
		member := callee
		if fr.t.Kind(member) == syntax.BracketExpr {
			member = syntax.NodeID(fr.t.Nodes[member].Lhs)
		}
		recv, c := fr.receiver(syntax.NodeID(fr.t.Nodes[member].Lhs), call.Callee)
		if c != nil {
			return nil, c
		}
		args, c := fr.args(argNodes, call)
		if c != nil {
			return nil, c
		}
		if call.Witness != 0 {
			return in.callValue(fr, deref(fr.cell(call.Witness).V), n, append([]Value{recv}, args...))
		}
		if in.r.Fn(call.Callee).Declared&sem.EffAsync != 0 {
			return &Future{Fn: call.Callee, Args: append([]Value{recv}, args...)}, nil
		}
		return in.callFnValue(fr, call.Callee, n, append(append(args, fr.witnessValues(call.Passes)...), fr.constValues(call.ConstArgs)...), recv)
	case sem.CallStatic:
		args, c := fr.args(argNodes, call)
		if c != nil {
			return nil, c
		}
		return in.callValue(fr, deref(fr.cell(call.Witness).V), n, args)
	case sem.CallFn, sem.CallAssoc, sem.CallOperator:
		args, c := fr.args(argNodes, call)
		if c != nil {
			return nil, c
		}
		if in.r.Fn(call.Callee).Declared&sem.EffAsync != 0 {
			return &Future{Fn: call.Callee, Args: args}, nil
		}
		return in.callFnValue(fr, call.Callee, n, append(append(args, fr.witnessValues(call.Passes)...), fr.constValues(call.ConstArgs)...), nil)
	}
	fv, c := fr.expr(callee)
	if c != nil {
		return nil, c
	}
	args, c := fr.args(argNodes, call)
	if c != nil {
		return nil, c
	}
	return in.callValue(fr, deref(fv), n, args)
}

// callValue calls a function-typed value.
func (in *Interp) callValue(fr *frame, fv Value, n syntax.NodeID, args []Value) (Value, *ctrl) {
	switch f := fv.(type) {
	case *FnItem:
		if in.r.Fn(f.Fn).Declared&sem.EffAsync != 0 {
			return &Future{Fn: f.Fn, Args: args}, nil
		}
		return in.callFnValue(fr, f.Fn, n, args, nil)
	case *Partial:
		return in.callFnValue(fr, f.Fn, n, append(append([]Value{}, args...), f.Bound...), nil)
	case *BoundMethod:
		if in.r.Fn(f.Fn).Declared&sem.EffAsync != 0 {
			return &Future{Fn: f.Fn, Args: append([]Value{f.Recv}, args...)}, nil
		}
		return in.callFnValue(fr, f.Fn, n, args, f.Recv)
	case *Closure:
		return in.callClosure(fr, f, n, args)
	}
	fr.panicAt(n, "value is not callable")
	return nil, nil
}

func (fr *frame) callUser(fn sem.EntityID, n syntax.NodeID, args []Value, recv Value) (Value, *ctrl) {
	return fr.in.callFnValue(fr, fn, n, args, recv)
}

// callFnValue calls a named function: a body, a std builtin, or an
// interface requirement dispatched on the receiver's dynamic type.
func (in *Interp) callFnValue(fr *frame, fn sem.EntityID, n syntax.NodeID, args []Value, recv Value) (Value, *ctrl) {
	e := in.r.Entity(fn)
	info := in.r.Fn(fn)
	if e.Kind == sem.EntFn && in.r.Entity(e.Parent).Kind == sem.EntInterface {
		return in.dispatch(fr, fn, n, args, recv)
	}
	if info.Abi != "" || info.Naked {
		fr.panicAt(n, "foreign function `%s` needs `kigumi build`", e.Name)
	}
	if info.Body != 0 && e.Flags&sem.EfStd != 0 {
		if b, ok := in.accels[in.builtinKey(fn)]; ok && (!in.NoAccel || in.required[in.builtinKey(fn)]) {
			if recv != nil {
				args = append([]Value{recv}, args...)
			}
			return b(in, fr, n, args)
		}
	}
	if info.Body == 0 {
		key := in.builtinKey(fn)
		b, ok := in.builtins[key]
		if !ok {
			fr.panicAt(n, "no runtime for `%s`", key)
		}
		if recv != nil {
			args = append([]Value{recv}, args...)
		}
		return b(in, fr, n, args)
	}
	callee := in.newFrame(fn, e.File)
	callee.caller = fr
	callee.pushScope()
	// A method reached as a plain function value carries its receiver as
	// the first argument.
	if info.SelfParam != 0 && recv == nil && len(args) > 0 {
		recv, args = args[0], args[1:]
	}
	if info.SelfParam != 0 {
		if info.Recv == sem.RecvMove && !in.isDropFn(fn) {
			callee.declare(info.SelfParam, recv)
		} else {
			callee.cells[info.SelfParam] = &Cell{V: recv}
		}
	}
	for i, p := range info.Params {
		if i < len(args) {
			callee.declare(p, args[i])
		}
	}
	for i, w := range info.Witnesses {
		if j := len(info.Params) + i; j < len(args) {
			callee.cells[w.Local] = &Cell{V: args[j]}
		}
	}
	for i, cp := range info.ConstParams {
		if j := len(info.Params) + len(info.Witnesses) + i; j < len(args) {
			callee.cells[cp] = &Cell{V: args[j]}
		}
	}
	return in.runBody(callee, info.Body, in.r.Types.Node(info.Sig).Elem)
}

func (in *Interp) callClosure(fr *frame, cl *Closure, n syntax.NodeID, args []Value) (Value, *ctrl) {
	info := in.r.Closure(cl.Ent)
	e := in.r.Entity(cl.Ent)
	callee := in.newFrame(e.Parent, e.File)
	callee.pushScope()
	for ent, c := range cl.Env {
		callee.cells[ent] = c
	}
	for i, p := range info.Params {
		if i < len(args) {
			callee.declare(p, args[i])
		}
	}
	body := syntax.NodeID(callee.t.Nodes[e.Node].Rhs)
	return in.runBody(callee, body, info.Ret)
}
