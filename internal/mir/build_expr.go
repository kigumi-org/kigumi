package mir

import (
	"kigumi/internal/hir"
	"kigumi/internal/sem"
)

// expr lowers an expression to a local holding its value, with the
// checker's coercion steps applied.
func (b *builder) expr(e *hir.Expr) LocalID {
	v := b.exprRaw(e)
	// A fallback lifts only its hit value (build_ops.go); the miss arm is
	// already at the lifted type.
	if e.Kind == hir.Fallback {
		return v
	}
	return b.coerce(e.Coercion, e.Node, v)
}

func (b *builder) exprRaw(e *hir.Expr) LocalID {
	n, t := e.Node, e.Type
	switch e.Kind {
	case hir.Local:
		id := b.local(e.Ent)
		if b.fn.Locals[id].Cell {
			return b.emit(Inst{Op: OpCellGet, Dst: b.temp(b.r.Entity(e.Ent).Type), Args: []LocalID{id}, Node: n})
		}
		return id
	case hir.Lit:
		return b.emit(Inst{Op: OpConst, Dst: b.temp(t), Lit: e.Lit, Type: t, Node: n})
	case hir.FnItem:
		async := b.r.Fn(e.Ent).Declared&sem.EffAsync != 0
		return b.emit(Inst{Op: OpFnItem, Dst: b.temp(t), Ent: e.Ent, Async: async, Node: n})
	case hir.Host:
		return b.emit(Inst{Op: OpBuiltin, Dst: b.temp(t), Str: "host", Node: n})
	case hir.Wrap:
		return b.expr(e.Args[0])
	case hir.Unary:
		v := b.expr(e.Args[0])
		if e.Ent != 0 {
			return b.emit(Inst{Op: OpCall, Dst: b.temp(t), Ent: e.Ent, Args: []LocalID{v}, Node: n})
		}
		return b.emit(Inst{Op: OpUnary, Dst: b.temp(t), Str: e.Name, Args: []LocalID{v}, Node: n})
	case hir.Binary:
		l := b.expr(e.Args[0])
		r := b.expr(e.Args[1])
		if e.Ent != 0 {
			return b.emit(Inst{Op: OpCall, Dst: b.temp(t), Ent: e.Ent, Args: []LocalID{l, r}, Node: n})
		}
		return b.emit(Inst{Op: OpBinary, Dst: b.temp(t), Str: e.Name, Args: []LocalID{l, r}, Node: n})
	case hir.And:
		return b.shortAnd(e)
	case hir.Fallback:
		return b.fallback(e)
	case hir.Pipe:
		return b.pipe(e)
	case hir.Is:
		v := b.expr(e.Args[0])
		return b.isExpr(e.Pat, v)
	case hir.Call, hir.Intrinsic, hir.Variant, hir.MethodCall, hir.StaticCall, hir.ValueCall:
		return b.call(e)
	case hir.Index:
		base := b.expr(e.Args[0])
		idx := b.expr(e.Args[1])
		return b.emit(Inst{Op: OpIndex, Dst: b.temp(t), Args: []LocalID{base, idx}, Node: n})
	case hir.Field:
		base := b.expr(e.Args[0])
		return b.emit(Inst{Op: OpField, Dst: b.temp(t), Args: []LocalID{base}, Index: e.Index, Node: n})
	case hir.FieldMove:
		base := b.expr(e.Args[0])
		return b.emit(Inst{Op: OpFieldMove, Dst: b.temp(t), Args: []LocalID{base}, Index: e.Index, Node: n})
	case hir.MethodValue:
		recv := b.expr(e.Args[0])
		async := b.r.Fn(e.Ent).Declared&sem.EffAsync != 0
		return b.emit(Inst{Op: OpClosure, Dst: b.temp(t), Ent: e.Ent, Args: []LocalID{recv}, Str: "method", Async: async, Node: n})
	case hir.OptField:
		return b.optField(e)
	case hir.Try:
		return b.try(e)
	case hir.Record:
		return b.recordLit(e)
	case hir.Lambda:
		return b.lambda(e)
	case hir.BlockExpr:
		return b.blockExpr(e.Block)
	case hir.If:
		return b.ifExpr(e)
	case hir.IfLet:
		return b.ifLet(e)
	case hir.Match:
		return b.matchExpr(e)
	case hir.Loop:
		return b.forExpr(e)
	case hir.Return:
		var v LocalID
		if len(e.Args) == 0 {
			v = b.unit(n)
			// A checker coercion has no value node to attach to here, so a
			// bare `return` in a fallible-Unit function is Ok-wrapped
			// directly against the function's own return type.
			if val, _, ok := b.r.Types.IsResult(b.ret); ok && val == sem.TyUnit {
				v = b.emit(Inst{Op: OpVariant, Dst: b.temp(b.ret), Ent: b.variantNamed(b.r.Types.ResultEnt(), "Ok"), Type: b.ret, Args: []LocalID{v}, Node: n})
			}
		} else {
			v = b.exprMoved(e.Args[0])
		}
		b.unwindTo(0, false)
		b.returnValue(v)
		return b.never(n)
	case hir.Fail:
		// The error leaves every scope, so a named local travels in a
		// temporary before the scopes drop it.
		v := b.exprMoved(e.Args[0])
		b.failWith(n, v)
		return b.never(n)
	case hir.Break, hir.Continue:
		l := b.loops[len(b.loops)-1]
		var v LocalID
		if e.Kind == hir.Break && len(e.Args) > 0 {
			v = b.exprMoved(e.Args[0])
		}
		b.unwindTo(l.depth, false)
		if e.Kind == hir.Break {
			if l.valued {
				b.assign(l.result, v)
			}
			b.jump(l.exit)
		} else {
			b.jump(l.cont)
		}
		return b.never(n)
	case hir.Asm:
		return b.asm(e)
	case hir.Await:
		// await consumes the future, so it moves like Fail above.
		fut := b.exprMoved(e.Args[0])
		return b.emit(Inst{Op: OpBuiltin, Dst: b.temp(t), Str: "await", Args: []LocalID{fut}, Node: n})
	case hir.Allocator:
		// The guard pops the allocator when it is dropped, so every exit
		// from the block, structured or not, restores the previous one.
		scope := b.expr(e.Args[0])
		b.pushScope()
		b.own(b.emit(Inst{Op: OpBuiltin, Dst: b.temp(e.Args[0].Type), Str: "alloc.push", Args: []LocalID{scope}, Node: n}))
		result := b.detach(b.blockExpr(e.Block))
		b.exitScope()
		return result
	case hir.Borrow:
		// A scalar `&mut` of a bare local/parameter aliases that
		// local's own cell (sem flags it EfAddrTaken so it gets one) rather
		// than boxing a disconnected copy, so the write reaches whatever
		// holds this borrow, not just reads through it.
		if e.Mut {
			if _, ok := b.r.ScalarRefElem(t); ok && e.Args[0].Kind == hir.Local {
				if id := b.local(e.Args[0].Ent); b.fn.Locals[id].Cell {
					return b.emit(Inst{Op: OpBorrow, Dst: b.temp(t), Args: []LocalID{id}, Index: 1, Node: n})
				}
			}
		}
		v := b.expr(e.Args[0])
		mut := 0
		if e.Mut {
			mut = 1
		}
		ref := b.emit(Inst{Op: OpBorrow, Dst: b.temp(t), Args: []LocalID{v}, Index: mut, Node: n})
		// A scalar `&mut` must be stored through by replacing its
		// referent (Copy scalars alias their obj on plain copy), so it is
		// boxed; the box takes its own reference first since a borrow
		// doesn't own what it points at, but the cell's store releases
		// what it replaces.
		if e.Mut {
			if _, ok := b.r.ScalarRefElem(t); ok {
				owned := b.emit(Inst{Op: OpShare, Dst: b.temp(t), Args: []LocalID{ref}, Node: n, Copy: true})
				return b.emit(Inst{Op: OpNewCell, Dst: b.temp(t), Args: []LocalID{owned}, Node: n})
			}
		}
		return ref
	case hir.Interp:
		// The result temporary is numbered before the pieces it interpolates.
		in := Inst{Op: OpInterp, Dst: b.temp(sem.TyString), Strs: e.Strs, Node: n}
		for _, a := range e.Args {
			in.Args = append(in.Args, b.expr(a))
		}
		return b.emit(in)
	case hir.Shell:
		in := Inst{Op: OpShell, Dst: b.temp(t), Strs: e.Strs, Node: n}
		for _, a := range e.Args {
			in.Args = append(in.Args, b.expr(a))
		}
		return b.emit(in)
	case hir.Comptime:
		return b.comptime(e)
	case hir.Unit:
		return b.unit(n)
	}
	return b.emit(Inst{Op: OpPanic, Dst: b.temp(t), Str: e.Name, Node: n})
}
