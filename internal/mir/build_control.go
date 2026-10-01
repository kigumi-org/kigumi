package mir

import (
	"kigumi/internal/hir"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

func (b *builder) blockExpr(blk *hir.Block) LocalID {
	b.pushScope()
	for _, s := range blk.Stmts {
		b.stmt(s)
	}
	var result LocalID
	if blk.Tail != nil {
		result = b.detach(b.exprMoved(blk.Tail))
	}
	b.exitScope()
	if blk.Tail == nil {
		result = b.unit(blk.Node)
	}
	return b.coerce(blk.Coercion, blk.Node, result)
}

func (b *builder) ifExpr(e *hir.Expr) LocalID {
	cond := b.expr(e.Cond)
	result := b.temp(e.Type)
	then, els, join := b.newBlockAt(), b.newBlockAt(), b.newBlockAt()
	b.term(Term{Op: TermBranch, Args: []LocalID{cond}, Targets: []BlockID{then, els}})
	b.cur = then
	b.assignValue(result, b.blockExpr(e.Then), e.Node)
	b.jump(join)
	b.cur = els
	if e.Else != nil {
		b.assignValue(result, b.expr(e.Else), e.Node)
	} else {
		b.assign(result, b.unit(e.Node))
	}
	b.jump(join)
	b.cur = join
	return result
}

// assignValue stores a branch's value into the result of an `if`. A branch
// of an `if` used as a statement may still produce a value (a foreign call
// whose pointer nobody reads); it is dropped, and the result stays Unit.
func (b *builder) assignValue(dst, src LocalID, n syntax.NodeID) {
	if b.fn.Locals[dst].Type == sem.TyUnit && b.fn.Locals[src].Type != sem.TyUnit && b.fn.Locals[src].Type != sem.TyNever {
		if b.r.Types.Kind(b.fn.Locals[src].Type) != sem.KRef {
			b.emit(Inst{Op: OpDrop, Args: []LocalID{src}, Node: n})
		}
		b.assign(dst, b.unit(n))
		return
	}
	b.assign(dst, src)
}

func (b *builder) ifLet(e *hir.Expr) LocalID {
	init := b.expr(e.Cond)
	b.pushScope()
	b.ownTemp(init)
	result := b.temp(e.Type)
	then, els, join := b.newBlockAt(), b.newBlockAt(), b.newBlockAt()
	matched := b.newBlockAt()
	cond := b.testPattern(e.Pat, init)
	b.term(Term{Op: TermBranch, Args: []LocalID{cond}, Targets: []BlockID{matched, els}})
	b.cur = matched
	b.pushScope()
	b.bindPattern(e.Pat, init)
	if e.Guard != nil {
		gv := b.expr(e.Guard)
		fail := b.newBlockAt()
		b.term(Term{Op: TermBranch, Args: []LocalID{gv}, Targets: []BlockID{then, fail}})
		b.cur = fail
		b.unwindScope(b.scopes[len(b.scopes)-1], false)
		b.jump(els)
	} else {
		b.jump(then)
	}
	b.cur = then
	b.assignValue(result, b.blockExpr(e.Then), e.Node)
	b.exitScope()
	b.jump(join)
	b.cur = els
	if e.Else != nil {
		b.assignValue(result, b.expr(e.Else), e.Node)
	} else {
		b.assign(result, b.unit(e.Node))
	}
	b.jump(join)
	b.cur = join
	b.exitScope()
	return result
}

func (b *builder) forExpr(e *hir.Expr) LocalID {
	n := e.Node
	pattern, head := e.Pat, e.Cond
	valued := e.Type != sem.TyUnit && e.Type != sem.TyNever
	var result LocalID
	if valued {
		result = b.temp(e.Type)
	}
	cond, loop, next, exit := b.newBlockAt(), b.newBlockAt(), b.newBlockAt(), b.newBlockAt()
	// The loop's bookkeeping temporaries (and an iterable produced for the
	// loop) live in a scope of their own, so break, return and the normal
	// exit all release them.
	b.pushScope()
	var iter, idx, length LocalID
	var refHead bool
	if pattern != nil {
		// A borrowed head (its own type, `&Array[T]`/`&mut Array[T]`, or a
		// fresh `&`/`&mut` written at the head) is never moved nor owned by
		// the loop: only a plain owned head is, and a lent
		// `self`/`mut self` is excluded the same way declare() excludes it.
		refHead = b.r.Types.Kind(head.Type) == sem.KRef
		if lid, ok := b.locals[head.Ent]; !refHead && head.Kind == hir.Local && ok && !b.fn.Locals[lid].Borrowed {
			iter = b.exprMoved(head)
		} else {
			iter = b.expr(head)
		}
		if b.fn.Locals[iter].Ent == 0 {
			if refHead {
				b.ownBorrowedSource(iter)
			} else {
				b.own(iter)
			}
		}
		idx = b.own(b.emit(Inst{Op: OpConst, Dst: b.temp(sem.TyUsize), Lit: sem.Literal{Kind: sem.LitInt}, Type: sem.TyUsize, Str: "0", Node: n}))
		length = b.own(b.emit(Inst{Op: OpBuiltin, Dst: b.temp(sem.TyUsize), Str: "iter.len", Args: []LocalID{iter}, Node: n}))
	}
	b.jump(cond)
	b.cur = cond
	switch {
	case pattern != nil:
		c := b.emit(Inst{Op: OpBinary, Dst: b.temp(sem.TyBool), Str: "<", Args: []LocalID{idx, length}, Node: n})
		b.term(Term{Op: TermBranch, Args: []LocalID{c}, Targets: []BlockID{loop, exit}})
	case head != nil:
		c := b.expr(head)
		b.term(Term{Op: TermBranch, Args: []LocalID{c}, Targets: []BlockID{loop, exit}})
	default:
		b.jump(loop)
	}
	b.cur = loop
	b.loops = append(b.loops, &loopCtx{cont: next, exit: exit, depth: len(b.scopes), valued: valued, result: result})
	b.pushScope()
	if pattern != nil {
		// Copy keeps the retain-in-place iter.at (some std loops re-read
		// by index, e.g. Array.reverse); a borrowed head binds through
		// iter.at_ref instead of retaining; an owned element moves once
		// via iter.at_move.
		at := "iter.at"
		switch {
		case b.r.IsCopy(pattern.Type):
		case refHead:
			at = "iter.at_ref"
		default:
			at = "iter.at_move"
		}
		elem := b.emit(Inst{Op: OpBuiltin, Dst: b.temp(pattern.Type), Str: at, Args: []LocalID{iter, idx}, Node: n})
		b.bindPattern(pattern, elem)
	}
	b.blockExpr(e.Block)
	b.exitScope()
	b.loops = b.loops[:len(b.loops)-1]
	b.jump(next)
	b.cur = next
	if pattern != nil {
		one := b.emit(Inst{Op: OpConst, Dst: b.temp(sem.TyUsize), Lit: sem.Literal{Kind: sem.LitInt}, Type: sem.TyUsize, Str: "1", Node: n})
		sum := b.emit(Inst{Op: OpBinary, Dst: b.temp(sem.TyUsize), Str: "+", Args: []LocalID{idx, one}, Node: n})
		b.emit(Inst{Op: OpDrop, Args: []LocalID{idx}, Node: n})
		b.emit(Inst{Op: OpCopy, Dst: idx, Args: []LocalID{sum}, Node: n})
	}
	b.jump(cond)
	b.cur = exit
	b.exitScope()
	if e.Type == sem.TyNever {
		// Nothing breaks out of this loop; what follows never runs.
		b.term(Term{Op: TermUnreachable})
		return b.never(n)
	}
	if valued {
		return result
	}
	return b.unit(n)
}
