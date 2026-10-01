package mir

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// Borrow rules, checked per function on the MIR: a borrow may not
// escape (return, store, capture), the owner cannot be touched while an
// exclusive borrow is live, and cannot be mutated, moved or dropped while
// a shared borrow is live. Liveness is by last use.

type borrowInfo struct {
	owner    LocalID
	hasOwner bool
	mut      bool
	node     syntax.NodeID
	// path and exact are the owner-rooted field chain the borrow was taken
	// through (root-first) and whether it is a plain field projection; see
	// projectionPlace. A disjoint path lets two borrows of the same owner
	// coexist.
	path  []int
	exact bool
}

type borrowCheck struct {
	p       *Program
	f       *Func
	holders map[LocalID]*borrowInfo
	liveOut []map[LocalID]bool
	// definedIn holds the holders that may have been assigned on some
	// path into a block: a binding that `is` assigns only when its pattern
	// matches is live at the join but not yet defined before the test.
	definedIn []map[LocalID]bool
	reported  map[syntax.NodeID]bool
	// defs maps a temporary to its defining instruction (nil when defined
	// more than once), built on first use.
	defs map[LocalID]*Inst
	// chainOnly caches chainedOnly's result per local.
	chainOnly map[LocalID]bool
}

func analyzeBorrows(p *Program, f *Func) {
	c := &borrowCheck{p: p, f: f, holders: map[LocalID]*borrowInfo{}, reported: map[syntax.NodeID]bool{}}
	c.collect()
	if len(c.holders) == 0 {
		return
	}
	c.liveness()
	c.definitions()
	for bid := range f.Blocks {
		c.checkBlock(BlockID(bid))
	}
}

// collect finds the locals holding borrows: borrow results, copies of
// them, and reference-typed parameters.
func (c *borrowCheck) collect() {
	for _, prm := range c.f.Params {
		if c.p.R.Types.Kind(c.f.Locals[prm].Type) == sem.KRef {
			c.holders[prm] = &borrowInfo{mut: c.p.R.Types.Node(c.f.Locals[prm].Type).Flags&1 != 0}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, blk := range c.f.Blocks {
			for _, in := range blk.Insts {
				switch in.Op {
				case OpBorrow:
					if _, ok := c.holders[in.Dst]; !ok {
						if root, path, exact, ok := c.projectionPlace(in.Args[0]); ok {
							c.holders[in.Dst] = &borrowInfo{owner: root, hasOwner: true, mut: in.Index == 1, node: in.Node, path: path, exact: exact}
							changed = true
						}
					}
				case OpCopy, OpMove:
					if src, ok := c.holders[in.Args[0]]; ok {
						if _, has := c.holders[in.Dst]; !has {
							c.holders[in.Dst] = src
							changed = true
						}
					}
				case OpRecord, OpCall:
					if _, has := c.holders[in.Dst]; !has {
						if src, ok := c.lifetimeSource(in); ok {
							c.holders[in.Dst] = src
							changed = true
						}
					}
				}
			}
		}
	}
}

// checkBlock walks a block backward, tracking the holders live after each
// instruction, and reports the conflicts and escapes.
func (c *borrowCheck) checkBlock(bid BlockID) {
	blk := &c.f.Blocks[bid]
	live := map[LocalID]bool{}
	for l := range c.liveOut[bid] {
		live[l] = true
	}
	for _, a := range blk.Term.Args {
		if b, ok := c.holders[a]; ok {
			// A holder with no owner is a borrow inherited from a
			// parameter, not one taken of a local: returning it is the
			// intended lifetime escape, and the caller's
			// own call site becomes the new holder (lifetimeSource).
			if !b.hasOwner {
				live[a] = true
				continue
			}
			c.report(b.node, "borrow-escape", "a borrow cannot be returned")
			live[a] = true
		}
	}
	before := c.definedBefore(bid)
	for i := len(blk.Insts) - 1; i >= 0; i-- {
		in := blk.Insts[i]
		c.checkInst(in, intersect(live, before[i]))
		if _, ok := c.holders[in.Dst]; ok && in.Op != OpDrop && in.Op != OpSetField && in.Op != OpSetIndex && in.Op != OpCellSet && in.Op != OpBoxReplace {
			delete(live, in.Dst)
		}
		for _, a := range uses(in) {
			if _, ok := c.holders[a]; ok {
				live[a] = true
			}
		}
	}
}

// checkInst sees the holders live after in; a conflict is a touch of an
// owner whose borrow outlives this instruction.
func (c *borrowCheck) checkInst(in Inst, liveAfter map[LocalID]bool) {
	switch in.Op {
	case OpRecord, OpVariant, OpArray, OpClosure:
		if in.Op == OpClosure && c.p.R.Closure(in.Ent).Immediate {
			break
		}
		// A record that itself became a holder (lifetimeSource) is
		// routing a borrow through its own `['a]`, not letting it
		// escape: the check below runs on the record's own liveness
		// instead, exactly like an OpCopy of a holder.
		if in.Op == OpRecord {
			if _, ok := c.holders[in.Dst]; ok {
				break
			}
		}
		for _, a := range in.Args {
			if _, ok := c.holders[a]; ok {
				c.report(in.Node, "borrow-escape", "a borrow cannot be stored")
			}
		}
	case OpSetField, OpSetIndex, OpBoxReplace:
		if _, ok := c.holders[in.Args[len(in.Args)-1]]; ok {
			c.report(in.Node, "borrow-escape", "a borrow cannot be stored")
		}
	}
	for holder := range liveAfter {
		b := c.holders[holder]
		if !b.hasOwner || holder == in.Dst {
			continue
		}
		touched, path, exact := c.touch(in, b.owner)
		if !touched || !fieldPathsOverlap(path, b.path, exact, b.exact) {
			continue
		}
		mutates := in.Op == OpMove || in.Op == OpDrop || (in.Op == OpBorrow && in.Index == 1) ||
			in.Op == OpSetField || in.Op == OpSetIndex || in.Op == OpCellSet || in.Op == OpBoxReplace
		name := c.f.Locals[b.owner].Name
		switch {
		case in.Op == OpMove || in.Op == OpDrop:
			c.report(in.Node, "move-while-borrowed", "`"+name+"` is borrowed and cannot be moved or dropped here")
		case b.mut:
			c.report(in.Node, "borrow-conflict", "`"+name+"` is exclusively borrowed and cannot be used here")
		case mutates || in.Dst == b.owner:
			c.report(in.Node, "borrow-conflict", "`"+name+"` is borrowed and cannot be mutated here")
		}
	}
}
