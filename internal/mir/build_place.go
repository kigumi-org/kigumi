package mir

import (
	"kigumi/internal/diag"
	"kigumi/internal/hir"
	"kigumi/internal/sem"
)

// place is an assignment target whose operands have already been evaluated.
// Resolving once is what makes `arr[next()] += 1` call next() a single time and
// what orders base and index before the right-hand side.
type place struct {
	src  *hir.Place
	id   LocalID
	base LocalID
	idx  LocalID
}

func (b *builder) resolvePlace(pl *hir.Place) place {
	switch pl.Kind {
	case hir.PlaceField:
		return place{src: pl, base: b.expr(pl.Base)}
	case hir.PlaceIndex:
		base := b.expr(pl.Base)
		idx := b.expr(pl.Idx)
		return place{src: pl, base: base, idx: idx}
	}
	return place{src: pl, id: b.local(pl.Ent)}
}

func (b *builder) loadPlace(p place) LocalID {
	switch p.src.Kind {
	case hir.PlaceField:
		return b.emit(Inst{Op: OpField, Dst: b.temp(p.src.Type), Args: []LocalID{p.base}, Index: p.src.Index, Node: p.src.Node})
	case hir.PlaceIndex:
		return b.emit(Inst{Op: OpIndex, Dst: b.temp(p.src.Type), Args: []LocalID{p.base, p.idx}, Node: p.src.Node})
	}
	return b.expr(p.src.Expr)
}

// storePlace overwrites a place, reinitializing it: a field or
// element releases on the store itself, so a second drop would double-free;
// a named local is dropped through the local, not a copy, so the ownership
// pass sees a drop, not a read; a borrow rebinds the reference and owns
// nothing to drop.
func (b *builder) storePlace(p place, v LocalID) {
	switch p.src.Kind {
	case hir.PlaceField:
		b.emit(Inst{Op: OpSetField, Args: []LocalID{p.base, v}, Index: p.src.Index, Node: p.src.Node})
	case hir.PlaceIndex:
		b.emit(Inst{Op: OpSetIndex, Args: []LocalID{p.base, p.idx, v}, Node: p.src.Node})
	default:
		if b.fn.Locals[p.id].Borrowed {
			if b.storeSelf(p, v) {
				return
			}
			if b.canBoxReplace(b.fn.Locals[p.id].Type) {
				b.storeSelfReplace(p, v)
				return
			}
			b.rejectBorrowedReassign(p)
			return
		} else if _, ok := b.r.ScalarRefElem(b.fn.Locals[p.id].Type); b.fn.Locals[p.id].Cell || ok {
			b.emit(Inst{Op: OpCellSet, Args: []LocalID{p.id, v}, Node: p.src.Node})
			return
		}
		if b.r.Types.Kind(b.fn.Locals[p.id].Type) != sem.KRef {
			b.emit(Inst{Op: OpDrop, Args: []LocalID{p.id}, Node: p.src.Node})
		}
		b.emit(Inst{Op: OpCopy, Dst: p.id, Args: []LocalID{v}, Node: p.src.Node})
	}
}

// canBoxReplace reports whether t's runtime box is safe for storeSelfReplace's
// content swap. FormOpaque also reaches here (Bool, i64, Array, ...) but some
// of its values are immortal interned singletons (vm trueObj/falseObj, native
// rt_bool's static t/f) shared process-wide; swapping one's content would
// corrupt every other reference to it, so only ADT and resource receivers,
// whose boxes are privately owned, take this path.
func (b *builder) canBoxReplace(t sem.TypeID) bool {
	if b.r.Types.Kind(t) != sem.KNamed {
		return false
	}
	form := b.r.TypeDecl(b.r.Types.Node(t).Ent).Form
	return form == sem.FormAdt || form == sem.FormResource
}

// rejectBorrowedReassign reports `self = newValue` for a receiver shape
// storeSelf can't field-copy and storeSelfReplace can't box-swap safely.
func (b *builder) rejectBorrowedReassign(p place) {
	t := b.r.Tree(b.fn.File)
	msg := "self cannot be reassigned as a whole for this receiver; only a multi-variant ADT or a resource supports it"
	b.fn.Diags = append(b.fn.Diags, diag.Diagnostic{Severity: diag.Error, Loc: diag.At(t.File, t.Span(p.src.Node)), Code: "borrowed-self-reassign", Msg: msg})
}

// storeSelfReplace overwrites self's box in place for receivers storeSelf
// can't field-copy (a multi-variant ADT, or a resource): unlike a field
// copy, this works without knowing self's fields, so it needs no shape
// match between the old and new value and runs the new value's own
// drop(), not a synthesized field-by-field one.
func (b *builder) storeSelfReplace(p place, v LocalID) {
	dst := p.id
	if b.fn.Locals[p.id].Cell {
		dst = b.emit(Inst{Op: OpCellGet, Dst: b.temp(b.fn.Locals[p.id].Type), Args: []LocalID{p.id}, Node: p.src.Node})
	}
	b.emit(Inst{Op: OpBoxReplace, Args: []LocalID{dst, v}, Node: p.src.Node})
}

// storeSelf field-copies into self's existing record instead of rebinding,
// so a Cell-backed self keeps identity via OpCellGet (no copy); rebinding
// via OpCellSet would double-fire drop(move self) on the new value before
// the assign.
func (b *builder) storeSelf(p place, v LocalID) bool {
	t := b.fn.Locals[p.id].Type
	if b.r.Types.Kind(t) != sem.KNamed {
		return false
	}
	info := b.r.TypeDecl(b.r.Types.Node(t).Ent)
	if info.Form != sem.FormRecord {
		return false
	}
	dst := p.id
	if b.fn.Locals[p.id].Cell {
		dst = b.emit(Inst{Op: OpCellGet, Dst: b.temp(t), Args: []LocalID{p.id}, Node: p.src.Node})
	}
	for _, fe := range info.Fields {
		fi := b.r.Field(fe)
		fv := b.emit(Inst{Op: OpField, Dst: b.temp(fi.Type), Args: []LocalID{v}, Index: fi.Index, Node: p.src.Node})
		b.emit(Inst{Op: OpSetField, Args: []LocalID{dst, fv}, Index: fi.Index, Node: p.src.Node})
	}
	if !b.r.IsCopy(t) {
		moved := b.emit(Inst{Op: OpMove, Dst: b.temp(t), Args: []LocalID{v}, Node: p.src.Node})
		b.emit(Inst{Op: OpDrop, Args: []LocalID{moved}, Node: p.src.Node})
	}
	return true
}
