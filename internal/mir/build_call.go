package mir

import (
	"kigumi/internal/hir"
	"kigumi/internal/sem"
)

func (b *builder) call(e *hir.Expr) LocalID {
	n, t := e.Node, e.Type
	switch e.Kind {
	case hir.Intrinsic:
		args := b.args(e, e.Args, true, 0)
		return b.emit(Inst{Op: OpBuiltin, Dst: b.temp(t), Str: e.Name, Args: args, Node: n})
	case hir.Variant:
		args := b.args(e, e.Args, true, 0)
		return b.emit(Inst{Op: OpVariant, Dst: b.temp(t), Ent: e.Ent, Type: t, Args: args, Node: n})
	case hir.MethodCall:
		rest := b.args(e, e.Args[1:], true, e.Ent)
		recv := b.receiver(e.Args[0], e.Ent)
		args := append([]LocalID{recv}, rest...)
		if e.Async {
			fnv := b.emit(Inst{Op: OpFnItem, Dst: b.temp(b.r.Fn(e.Ent).Sig), Ent: e.Ent, Node: n})
			return b.emit(Inst{Op: OpBuiltin, Dst: b.temp(t), Str: "future", Args: append([]LocalID{fnv}, args...), Node: n})
		}
		if e.Witness != 0 {
			return b.emit(Inst{Op: OpCallValue, Dst: b.temp(t), Ent: e.Ent, Args: append([]LocalID{b.witnessLocal(e.Witness)}, args...), Node: n})
		}
		args = append(args, b.witnessArgs(e.Passes, n)...)
		args = append(args, b.constArgs(e.ConstArgs, n)...)
		return b.emit(Inst{Op: OpCall, Dst: b.temp(t), Ent: e.Ent, Args: args, Node: n})
	case hir.StaticCall:
		args := b.args(e, e.Args, true, e.Ent)
		return b.emit(Inst{Op: OpCallValue, Dst: b.temp(t), Args: append([]LocalID{b.witnessLocal(e.Witness)}, args...), Node: n})
	case hir.Call:
		// A reborrowed arg would outlive the call it reborrows for when the
		// call is async: the future can cross `await` or be sent to a task,
		// so async args are always moved, never reborrowed.
		args := b.args(e, e.Args, !e.Async, e.Ent)
		if e.Async {
			fnv := b.emit(Inst{Op: OpFnItem, Dst: b.temp(b.r.Fn(e.Ent).Sig), Ent: e.Ent, Node: n})
			return b.emit(Inst{Op: OpBuiltin, Dst: b.temp(t), Str: "future", Args: append([]LocalID{fnv}, args...), Node: n})
		}
		args = append(args, b.witnessArgs(e.Passes, n)...)
		args = append(args, b.constArgs(e.ConstArgs, n)...)
		return b.emit(Inst{Op: OpCall, Dst: b.temp(t), Ent: e.Ent, Args: args, Node: n})
	}
	fv := b.expr(e.Args[0])
	args := append([]LocalID{fv}, b.args(e, e.Args[1:], true, 0)...)
	if ct := b.fn.Locals[fv].Type; b.r.Types.IsCFn(ct) {
		return b.emit(Inst{Op: OpCallC, Dst: b.temp(t), Type: ct, Args: args, Node: n})
	}
	return b.emit(Inst{Op: OpCallValue, Dst: b.temp(t), Args: args, Node: n})
}

// A bare `&mut T` local passed where `&mut T` is expected reborrows
// instead of moving, since the callee's borrow ends when the call
// returns; reborrow is false for an async call, whose future can outlive
// the call.
func (b *builder) argValue(a *hir.Expr, idx int, ent sem.EntityID, reborrow bool) LocalID {
	if reborrow && a.Kind == hir.Local && b.r.Types.Kind(a.Type) == sem.KRef && !b.r.IsCopy(a.Type) && b.declaredRef(ent, idx) {
		return b.expr(a)
	}
	return b.exprMoved(a)
}

// declaredRef is checked pre-instantiation: a type parameter unified to
// `&mut U` isn't written as `&mut T` in the signature, so E220 misses the
// escaping reborrow otherwise. ent 0 (no fixed callee) falls back
// to an argument-type-only check.
func (b *builder) declaredRef(ent sem.EntityID, idx int) bool {
	if ent == 0 {
		return true
	}
	params := b.r.Fn(ent).Params
	if idx < 0 || idx >= len(params) {
		return false
	}
	return b.r.Types.Kind(b.r.Entity(params[idx]).Type) == sem.KRef
}

// receiver lowers a method receiver by its kind: `mut self` borrows the
// place, `move self` moves the value, `self` shares it.
func (b *builder) receiver(base *hir.Expr, m sem.EntityID) LocalID {
	info := b.r.Fn(m)
	if info.Recv == sem.RecvMove {
		return b.exprMoved(base)
	}
	v := b.expr(base)
	if info.Recv == sem.RecvMut {
		return b.emit(Inst{Op: OpBorrow, Dst: b.temp(b.r.Types.Ref(base.Type, true)), Args: []LocalID{v}, Index: 1, Node: base.Node})
	}
	return v
}

func (b *builder) args(call *hir.Expr, exprs []*hir.Expr, reborrow bool, ent sem.EntityID) []LocalID {
	var out []LocalID
	for i, a := range exprs {
		out = append(out, b.argValue(a, i, ent, reborrow))
	}
	if call.Variadic {
		params := b.r.Types.Node(b.r.Fn(call.Ent).Sig).Args
		fixed := len(params) - 1
		if fixed < 0 {
			fixed = 0
		}
		if fixed > len(out) {
			fixed = len(out)
		}
		arr := b.emit(Inst{Op: OpArray, Dst: b.temp(params[len(params)-1]), Args: append([]LocalID{}, out[fixed:]...), Node: call.Node})
		out = append(out[:fixed], arr)
	}
	return out
}

func (b *builder) optField(e *hir.Expr) LocalID {
	n, t := e.Node, e.Type
	base := b.expr(e.Args[0])
	some := b.variantNamed(b.r.Types.OptionEnt(), "Some")
	isSome := b.emit(Inst{Op: OpIsVariant, Dst: b.temp(sem.TyBool), Ent: some, Args: []LocalID{base}, Node: n})
	result := b.temp(t)
	yes, no, join := b.newBlockAt(), b.newBlockAt(), b.newBlockAt()
	b.term(Term{Op: TermBranch, Args: []LocalID{isSome}, Targets: []BlockID{yes, no}})
	b.cur = yes
	payload := b.emit(Inst{Op: OpPayload, Dst: b.temp(b.r.Types.Node(e.Args[0].Type).Args[0]), Args: []LocalID{base}, Node: n})
	inner := b.emit(Inst{Op: OpField, Dst: b.temp(b.r.Types.Node(t).Args[0]), Args: []LocalID{payload}, Index: e.Index, Node: n})
	b.emit(Inst{Op: OpVariant, Dst: result, Ent: some, Type: t, Args: []LocalID{inner}, Node: n})
	b.jump(join)
	b.cur = no
	b.emit(Inst{Op: OpVariant, Dst: result, Ent: b.variantNamed(b.r.Types.OptionEnt(), "None"), Type: t, Node: n})
	b.jump(join)
	b.cur = join
	return result
}

func (b *builder) recordLit(e *hir.Expr) LocalID {
	info := b.r.TypeDecl(e.Ent)
	fields := make([]LocalID, len(info.Fields))
	set := make([]bool, len(info.Fields))
	for _, entry := range e.Entries {
		if !entry.Spread {
			fields[entry.Index] = b.exprMoved(entry.Value)
			set[entry.Index] = true
			continue
		}
		v := b.expr(entry.Value)
		for _, sf := range entry.Fields {
			fields[sf.Index] = b.emit(Inst{Op: OpField, Dst: b.temp(sf.Type), Args: []LocalID{v}, Index: sf.SrcIndex, Node: entry.Node})
			set[sf.Index] = true
		}
		srcType := entry.Value.Type
		if node := b.r.Types.Node(srcType); node.Kind == sem.KRef {
			srcType = node.Elem
		}
		if b.r.IsCopy(srcType) {
			// A Copy source (owned or borrowed) is unaffected: its fields
			// were read, not taken, so it stays usable.
			continue
		}
		// The spread source is consumed: its fields now belong to the new
		// record, so the source is moved and its shell released.
		moved := b.emit(Inst{Op: OpMove, Dst: b.temp(entry.Value.Type), Args: []LocalID{v}, Node: entry.Node})
		b.emit(Inst{Op: OpDrop, Args: []LocalID{moved}, Node: entry.Node})
	}
	for i := range fields {
		if !set[i] {
			fields[i] = b.unit(e.Node)
		}
	}
	return b.emit(Inst{Op: OpRecord, Dst: b.temp(e.Type), Ent: e.Ent, Type: e.Type, Args: fields, Node: e.Node})
}

// lambda lowers the closure body as its own function and captures the
// locals it uses (cells by reference, others by value).
func (b *builder) lambda(e *hir.Expr) LocalID {
	var caps []LocalID
	for _, cap := range e.Fn.Captures {
		id := b.local(cap.Ent)
		if cap.Cell || b.fn.Locals[id].Cell {
			caps = append(caps, id)
			continue
		}
		caps = append(caps, b.emit(Inst{Op: OpCopy, Dst: b.temp(b.fn.Locals[id].Type), Args: []LocalID{id}, Node: e.Node}))
	}
	if _, built := b.p.ByEnt[e.Ent]; !built {
		b.p.build(e.Fn)
	}
	return b.emit(Inst{Op: OpClosure, Dst: b.temp(e.Type), Ent: e.Ent, Args: caps, Node: e.Node})
}
