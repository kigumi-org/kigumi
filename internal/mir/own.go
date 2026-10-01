package mir

import (
	"kigumi/internal/syntax"
)

// Ownership states of a move-only local along a path.
type ownState uint8

const (
	ownUninit ownState = iota
	ownValid
	ownMoved
	ownConflict
	// ownMaybe joins assigned and never-assigned paths: a pattern binding
	// behind `&&`, which the checker only lets the matched path read.
	ownMaybe
)

// joinOwn merges two states at a block entry: a move on either side is a
// conflict, an unassigned side only makes the value conditional.
func joinOwn(a, b ownState) ownState {
	if a == b {
		return a
	}
	if a == ownMoved || b == ownMoved || a == ownConflict || b == ownConflict {
		return ownConflict
	}
	return ownMaybe
}

// dstate: val is whether the local still holds its value; field is whether
// a field read out of a root is still in the root. They are independent: a
// moved-out field's temporary stays valid in val until consumed.
type dstate struct {
	val, field map[LocalID]ownState
}

func newDstate() *dstate {
	return &dstate{val: map[LocalID]ownState{}, field: map[LocalID]ownState{}}
}

func cloneD(s *dstate) *dstate {
	return &dstate{val: cloneState(s.val), field: cloneState(s.field)}
}

// analyzeOwnership runs a forward dataflow over the CFG: every use needs a
// valid local, moves consume, drops of moved locals are removed, and
// joins must agree. A field moved out of an owned place
// keeps its own presence state independent of the place that held it;
// using that place as a whole while one of its fields is gone is also an
// error, and a drop of it becomes a drop of the fields still there.
func analyzeOwnership(p *Program, f *Func) {
	moveOnly := func(l Local) bool {
		return !p.R.IsCopy(l.Type) && !l.Cell
	}
	tracked := map[LocalID]bool{}
	for i, l := range f.Locals {
		if l.Ent != 0 && moveOnly(l) {
			tracked[LocalID(i)] = true
		}
	}
	pl := findPlaces(p, f)
	for l := range pl.key {
		if moveOnly(f.Locals[l]) {
			tracked[l] = true
		}
	}
	if len(tracked) == 0 {
		return
	}
	a := &owner{p: p, f: f, pl: pl, tracked: tracked, in: make([]*dstate, len(f.Blocks)),
		reported: map[syntax.NodeID]bool{}, defNode: map[LocalID]syntax.NodeID{}}
	entry := newDstate()
	for _, prm := range f.Params {
		if tracked[prm] {
			entry.val[prm] = ownValid
		}
	}
	a.in[0] = entry
	work := []BlockID{0}
	for len(work) > 0 {
		bid := work[0]
		work = work[1:]
		out := a.transfer(bid, cloneD(a.in[bid]), false)
		for _, succ := range f.Blocks[bid].Term.Targets {
			if a.merge(succ, out) {
				work = append(work, succ)
			}
		}
	}
	for bid := range f.Blocks {
		if a.in[bid] != nil {
			a.transfer(BlockID(bid), cloneD(a.in[bid]), true)
		}
	}
}

// transfer walks a block; in report mode it emits diagnostics and rewrites
// drops of consumed locals into no-ops or, of a partially moved place,
// into drops of the fields still there.
func (a *owner) transfer(bid BlockID, st *dstate, report bool) *dstate {
	blk := &a.f.Blocks[bid]
	for i := range blk.Insts {
		in := &blk.Insts[i]
		if in.Dst != 0 {
			a.defNode[in.Dst] = in.Node
		}
		switch in.Op {
		case OpDrop:
			l := a.pl.canon(in.Args[0])
			if !a.tracked[l] {
				continue
			}
			// A value the local may or may not hold keeps its drop: the
			// backends store null into every local first and into a local a
			// move emptied, so dropping an absent value is a no-op. Only a
			// drop that can never find a value goes.
			if st.val[l] == ownUninit || st.val[l] == ownMoved {
				if report {
					in.Op = OpNop
					in.Args = nil
				}
				continue
			}
			// A partially moved place still gets a plain drop: every
			// backend's free already walks an aggregate's fields and skips
			// a moved-out (null) one, so the fields still there are
			// released the same way a whole value's would be.
			st.val[l] = ownUninit
			continue
		case OpMove:
			src := a.pl.canon(in.Args[0])
			if a.tracked[src] {
				a.check(in, src, st.val, report)
				a.checkWhole(in.Node, src, st, report)
				st.val[src] = ownMoved
			}
		default:
			for argi, arg := range in.Args {
				l := a.pl.canon(arg)
				if !a.tracked[l] {
					continue
				}
				a.check(in, l, st.val, report)
				if a.pl.isPlace(arg) && a.p.consumesArg(in, argi) {
					a.checkWhole(in.Node, l, st, report)
					st.val[l] = ownMoved
				}
			}
		}
		if in.Op == OpField || in.Op == OpFieldMove {
			if l := a.pl.canon(in.Dst); a.tracked[l] {
				a.check(in, l, st.val, report)
				a.check(in, l, st.field, report)
				if in.Op == OpFieldMove {
					st.field[l] = ownMoved
				}
			}
		}
		if in.Op == OpSetField {
			if l, ok := a.pl.stored(a.f, *in); ok {
				st.val[l], st.field[l] = ownValid, ownValid
			}
		}
		if a.pl.isPlace(in.Dst) {
			if l := a.pl.canon(in.Dst); st.val[l] == ownUninit {
				st.val[l] = ownValid
			}
			continue
		}
		if in.Op != OpDrop && in.Op != OpSetField && in.Op != OpSetIndex && in.Op != OpCellSet && in.Op != OpBoxReplace && a.tracked[in.Dst] {
			st.val[in.Dst] = ownValid
		}
	}
	if blk.Term.Op == TermReturn {
		for _, arg := range blk.Term.Args {
			l := a.pl.canon(arg)
			if a.tracked[l] {
				a.check(nil, l, st.val, report)
				a.checkWhole(a.defNode[arg], l, st, report)
			}
		}
		a.checkBorrowedWhole(st, report)
	}
	return st
}
