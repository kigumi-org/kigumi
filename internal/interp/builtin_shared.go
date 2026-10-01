package interp

import "kigumi/internal/syntax"

// sharedBox is a Shared cell with its strong count; weak handles see the
// box after the last Shared dropped the content. borrow is a RefCell-style
// dynamic borrow flag (0 free, -1 exclusive, >0 shared count) since Shared's
// receiver is a freely-copyable handle the static borrow checker does not
// track.
type sharedBox struct {
	cell   *Cell
	strong int
	alive  bool
	borrow int
}

func registerShared(in *Interp) {
	s := "std/alloc."
	boxOf := func(v Value) *sharedBox { return deref(v).(*Opaque).Data.(*sharedBox) }
	in.def(s+"AllocatorHandle.allocated", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return Int{V: 0, T: fr.retType(n)}, nil
	})
	in.def(s+"Shared.new", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Shared", &sharedBox{cell: &Cell{V: deref(a[0])}, strong: 1, alive: true}), nil
	})
	in.def(s+"Shared.clone", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		b := boxOf(a[0])
		b.strong++
		return opaque("Shared", b), nil
	})
	in.def(s+"Shared.get", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return cloneValue(boxOf(a[0]).cell.V), nil
	})
	in.def(s+"Shared.set", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		cell := boxOf(a[0]).cell
		fr.dropOld(cell.V)
		cell.V = deref(a[1])
		return Unit{}, nil
	})
	in.def(s+"Shared.with", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		b := boxOf(a[0])
		if b.borrow < 0 {
			fr.panicAt(n, "Shared is exclusively borrowed")
		}
		b.borrow++
		defer func() { b.borrow-- }()
		return in.callValue(fr, a[1], n, []Value{&Ref{Cell: b.cell}})
	})
	in.def(s+"Shared.withMut", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		b := boxOf(a[0])
		if b.borrow != 0 {
			fr.panicAt(n, "Shared is already borrowed")
		}
		b.borrow = -1
		defer func() { b.borrow = 0 }()
		return in.callValue(fr, a[1], n, []Value{&Ref{Cell: b.cell}})
	})
	in.def(s+"Shared.downgrade", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Weak", boxOf(a[0])), nil
	})
	in.def(s+"Weak.clone", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return deref(a[0]), nil
	})
	in.def(s+"Weak.upgrade", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		b := boxOf(a[0])
		if !b.alive {
			return mkNone(in, fr.retType(n)), nil
		}
		b.strong++
		return mkSome(in, fr.retType(n), opaque("Shared", b)), nil
	})
}

// dropShared releases one strong reference; the last one drops the content.
func (fr *frame) dropShared(b *sharedBox) {
	b.strong--
	if b.strong > 0 {
		return
	}
	b.alive = false
	if fr.in.moveOnly(b.cell.V) {
		fr.dropDeep(b.cell.V)
	}
	b.cell.V = Moved{}
}
