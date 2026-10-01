package mir

import "slices"

// Point addresses the moment right after instruction Inst of block Block;
// Inst == len(insts) is the terminator.
type Point struct{ Block, Inst int }

// use is where a temporary was last read.
type use struct{ block, inst int }

// Plan is the runtime ownership plan of one function, shared by every
// backend that executes MIR: where unconsumed temporaries are released,
// how often each temporary is defined, and which alias temporaries must
// take their own reference.
type Plan struct {
	Releases    map[Point][]LocalID
	Defs        map[LocalID]int
	Materialize map[LocalID]bool
}

// MIR drops only named locals, so a temporary that no instruction consumes
// (stores, moves or hands to an owning callee) is released after its last
// use, provided that use sits in the block that defined it.
func (p *Program) planFunc(f *Func) *Plan {
	defBlock := map[LocalID]int{}
	defs := map[LocalID]int{}
	last := map[LocalID]use{}
	consumed := map[LocalID]bool{}
	noRelease := map[LocalID]bool{}
	borrowed := map[LocalID]LocalID{}
	isTemp := func(l LocalID) bool { return f.Locals[l].Ent == 0 }
	for b, blk := range f.Blocks {
		for _, in := range blk.Insts {
			if isTemp(in.Dst) && in.Op != OpNop && in.Op != OpDrop && in.Op != OpSetField && in.Op != OpSetIndex && in.Op != OpCellSet && in.Op != OpBoxReplace {
				defBlock[in.Dst] = b
			}
		}
	}
	// Block numbers do not follow execution order (loop bodies precede
	// their headers), so a temporary used outside its defining block is
	// left alone rather than released at a textual "last" use.
	for b, blk := range f.Blocks {
		for k, in := range blk.Insts {
			for i, a := range in.Args {
				if !isTemp(a) {
					continue
				}
				if defBlock[a] != b {
					noRelease[a] = true
				}
				last[a] = use{b, k}
				if p.consumes(in, i) {
					consumed[a] = true
				}
			}
			if in.Op == OpNop || !isTemp(in.Dst) {
				continue
			}
			switch in.Op {
			case OpDrop, OpSetField, OpSetIndex, OpCellSet, OpBoxReplace:
				continue
			}
			defs[in.Dst]++
			if _, used := last[in.Dst]; !used {
				last[in.Dst] = use{b, k}
			}
			// A copy of a named local only aliases it; a borrow is always an
			// alias, its source (a local, or a temporary kept alive by
			// extendBorrowed) is what gets released.
			if in.Op == OpCopy && len(in.Args) == 1 && (!isTemp(in.Args[0]) || noRelease[in.Args[0]]) {
				noRelease[in.Dst] = true
			}
			// iter.at_ref hands out a `for` element without
			// its own retain, so it needs no matching release either.
			if in.Op == OpBuiltin && in.Str == "iter.at_ref" {
				noRelease[in.Dst] = true
			}
			if in.Op == OpBorrow {
				noRelease[in.Dst] = true
				if len(in.Args) == 1 && isTemp(in.Args[0]) {
					borrowed[in.Dst] = in.Args[0]
				}
			}
		}
		for _, a := range blk.Term.Args {
			if isTemp(a) {
				if defBlock[a] != b {
					noRelease[a] = true
				}
				last[a] = use{b, len(blk.Insts)}
				if blk.Term.Op == TermReturn {
					consumed[a] = true
				}
			}
		}
	}
	extendBorrowed(borrowed, last, noRelease)
	plan := map[Point][]LocalID{}
	for t, u := range last {
		if consumed[t] || noRelease[t] || defs[t] != 1 || defBlock[t] != u.block {
			continue
		}
		plan[Point{u.block, u.inst}] = append(plan[Point{u.block, u.inst}], t)
	}
	for _, ts := range plan {
		slices.Sort(ts)
	}
	return &Plan{Releases: plan, Defs: defs, Materialize: p.planAliases(f, isTemp, defs)}
}

// extendBorrowed keeps a borrowed temporary alive as long as the borrow of
// it. A borrow is an alias, not a reference, so releasing the source at its
// own last use — the borrow instruction — would hand the callee a freed
// object. Chains are followed to a fixed point.
func extendBorrowed(borrowed map[LocalID]LocalID, last map[LocalID]use, noRelease map[LocalID]bool) {
	for changed := true; changed; {
		changed = false
		for alias, src := range borrowed {
			au, ok := last[alias]
			if !ok {
				continue
			}
			su := last[src]
			if au.block != su.block {
				if !noRelease[src] {
					noRelease[src] = true
					changed = true
				}
				continue
			}
			if au.inst > su.inst {
				last[src] = au
				changed = true
			}
		}
	}
}

// consumes reports whether argument i of an instruction takes over the
// value: it is stored, moved, boxed or handed to a callee that owns it.
// Reads, arithmetic, display and borrowed receivers do not.
func (p *Program) consumes(in Inst, i int) bool {
	if p.inPlace(in, i) {
		return false
	}
	switch in.Op {
	case OpField, OpFieldMove, OpIndex, OpIsVariant, OpIsType, OpPayload, OpUnbox,
		OpCellGet, OpBinary, OpUnary, OpInterp, OpBuiltin, OpBorrow:
		return false
	}
	return true
}
