package mir

import (
	"kigumi/internal/hir"
	"kigumi/internal/sem"
)

func (b *builder) pushScope() { b.scopes = append(b.scopes, &scope{}) }

// exitScope lowers the scope's defers and drops on the fall-through path.
func (b *builder) exitScope() {
	s := b.scopes[len(b.scopes)-1]
	b.scopes = b.scopes[:len(b.scopes)-1]
	b.unwindScope(s, false)
}

// unwindScope emits the cleanup of one scope: defers in reverse, then the
// drops of its owned locals in reverse declaration order. A defer body
// that moves one of those locals by value takes its final slot value, so
// the normal drop is skipped.
func (b *builder) unwindScope(s *scope, failing bool) {
	consumed := map[LocalID]bool{}
	for i := len(s.defers) - 1; i >= 0; i-- {
		d := s.defers[i]
		if d.Err && !failing {
			continue
		}
		fromBlock, fromInst := b.cur, len(b.block().Insts)
		if d.Block != nil {
			b.blockExpr(d.Block)
		} else {
			b.stmt(d.Body)
		}
		b.markMoved(fromBlock, fromInst, consumed)
	}
	for i := len(s.owned) - 1; i >= 0; i-- {
		if consumed[s.owned[i]] {
			continue
		}
		b.emit(Inst{Op: OpDrop, Args: []LocalID{s.owned[i]}})
	}
}

// markMoved records every local a just-lowered defer body (the
// instructions from fromBlock/fromInst through the current cursor) moves
// as a whole value, so unwindScope can skip its now-redundant drop.
func (b *builder) markMoved(fromBlock BlockID, fromInst int, consumed map[LocalID]bool) {
	for bid := fromBlock; bid <= b.cur; bid++ {
		insts := b.fn.Blocks[bid].Insts
		start := 0
		if bid == fromBlock {
			start = fromInst
		}
		for _, in := range insts[start:] {
			if in.Op == OpMove {
				consumed[in.Args[0]] = true
			}
		}
	}
}

// unwindTo emits the cleanups of every scope down to depth (exclusive),
// for early exits.
func (b *builder) unwindTo(depth int, failing bool) {
	for i := len(b.scopes) - 1; i >= depth; i-- {
		b.unwindScope(b.scopes[i], failing)
	}
}

// own registers a temporary with the innermost scope so it is dropped like
// a binding.
func (b *builder) own(l LocalID) LocalID {
	s := b.scopes[len(b.scopes)-1]
	s.owned = append(s.owned, l)
	return l
}

// ownBorrowedSource registers a for-loop's borrowed-head source for release
// when it is itself a temporary: a field/index/cell projection (rt_field,
// rt_index, rt_cellget) retains on read, and the alias itself has nothing
// to release (declare()'s KRef exclusion also keeps emit() from
// ever dropping the alias). A bare local/parameter source retains nothing
// and is left alone.
func (b *builder) ownBorrowedSource(iter LocalID) {
	insts := b.block().Insts
	if len(insts) == 0 {
		return
	}
	last := insts[len(insts)-1]
	if last.Op != OpBorrow || last.Dst != iter || len(last.Args) != 1 {
		return
	}
	if src := last.Args[0]; b.fn.Locals[src].Ent == 0 {
		b.own(src)
	}
}

// detach copies a named local into a temporary before the scope that owns
// the local is unwound, so the value returned outlives the drops.
func (b *builder) detach(v LocalID) LocalID {
	if b.fn.Locals[v].Ent == 0 {
		return v
	}
	return b.emit(Inst{Op: OpCopy, Dst: b.temp(b.fn.Locals[v].Type), Args: []LocalID{v}})
}

// declare makes a local for a binding entity; locals a closure assigns
// through live in cells. Every binding is owned by its scope and dropped
// at exit: for Copy values the drop only frees the backend's storage.
func (b *builder) declare(ent sem.EntityID) LocalID {
	e := b.r.Entity(ent)
	id := LocalID(len(b.fn.Locals))
	b.fn.Locals = append(b.fn.Locals, Local{Name: e.Name, Type: e.Type, Ent: ent, Cell: e.Flags&(sem.EfCapturedMut|sem.EfAddrTaken) != 0})
	b.locals[ent] = id
	// Borrows never own what they point at, so they are not dropped.
	if len(b.scopes) > 0 && b.r.Types.Kind(e.Type) != sem.KRef {
		s := b.scopes[len(b.scopes)-1]
		s.owned = append(s.owned, id)
	}
	return id
}

// param declares an ordinary parameter. One a closure assigns arrives as
// a plain value and is moved into a cell, as a `let` binding would be.
func (b *builder) param(d *hir.LocalDecl) LocalID {
	if b.r.Entity(d.Ent).Flags&(sem.EfCapturedMut|sem.EfAddrTaken) == 0 {
		return b.declare(d.Ent)
	}
	raw := b.temp(d.Type)
	id := b.declare(d.Ent)
	b.emit(Inst{Op: OpNewCell, Dst: id, Args: []LocalID{raw}})
	return raw
}

// local returns the local of a binding, declaring parameters of the
// enclosing function lazily.
func (b *builder) local(ent sem.EntityID) LocalID {
	if id, ok := b.locals[ent]; ok {
		return id
	}
	return b.declare(ent)
}
