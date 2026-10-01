package mir

// projectionPlace follows a temporary produced by reading into a value
// (a field, payload, element, unboxed value or copy) back to the named
// local it was read from, so a borrow of the part holds the whole. It also
// accumulates the field-index chain (root-first) crossed along the way, and
// whether every step was a plain field projection: an index, payload or
// unbox step marks the place inexact since the checker cannot tell which part of the
// owner it names, so it keeps treating it as the whole owner.
func (c *borrowCheck) projectionPlace(l LocalID) (root LocalID, path []int, exact bool, ok bool) {
	exact = true
	for {
		if c.f.Locals[l].Ent != 0 {
			return l, path, exact, true
		}
		in := c.definition(l)
		if in == nil || len(in.Args) == 0 {
			return 0, nil, false, false
		}
		switch in.Op {
		case OpField:
			path = append([]int{in.Index}, path...)
			l = in.Args[0]
		case OpPayload, OpIndex, OpUnbox:
			exact = false
			l = in.Args[0]
		case OpCopy:
			l = in.Args[0]
		default:
			return 0, nil, false, false
		}
	}
}

// chainedOnly reports whether every use of l is as the base of a further
// field/index/payload/unbox step, or as what a borrow takes directly (a
// mid-chain hop like `self.inner` on the way to `self.inner.y`, or to
// `&self.inner`). Such a hop is not itself a touch: the deeper step it
// feeds resolves the full path back to the root via projectionPlace (touch's
// own OpBorrow case for a direct borrow), and a prefix of an overlapping
// path always overlaps too, so checking only the deepest step is
// equivalent and avoids double-reporting one borrow.
func (c *borrowCheck) chainedOnly(l LocalID) bool {
	if c.chainOnly == nil {
		c.chainOnly = map[LocalID]bool{}
		total, chained := map[LocalID]int{}, map[LocalID]int{}
		count := func(a LocalID) { total[a]++ }
		for b := range c.f.Blocks {
			for _, in := range c.f.Blocks[b].Insts {
				for _, a := range in.Args {
					count(a)
				}
				switch in.Op {
				case OpField, OpSetField, OpIndex, OpPayload, OpUnbox, OpBorrow:
					if len(in.Args) > 0 {
						chained[in.Args[0]]++
					}
				}
			}
			for _, a := range c.f.Blocks[b].Term.Args {
				count(a)
			}
		}
		for id, n := range chained {
			if n > 0 && total[id] == n {
				c.chainOnly[id] = true
			}
		}
	}
	return c.chainOnly[l]
}

// definition finds the single instruction defining a temporary; nil when
// it has none or several.
func (c *borrowCheck) definition(l LocalID) *Inst {
	if c.defs == nil {
		c.defs = map[LocalID]*Inst{}
		for b := range c.f.Blocks {
			for i := range c.f.Blocks[b].Insts {
				in := &c.f.Blocks[b].Insts[i]
				if !shapes[in.Op].result || c.f.Locals[in.Dst].Ent != 0 {
					continue
				}
				if _, dup := c.defs[in.Dst]; dup {
					c.defs[in.Dst] = nil
				} else {
					c.defs[in.Dst] = in
				}
			}
		}
	}
	return c.defs[l]
}

// fieldPathsOverlap reports whether a borrow of a and a touch of b, both
// resolved from the same owner, can name overlapping storage: one path is a
// prefix of the other (equal counts as overlapping too), or either side is
// inexact and so stands for the whole owner.
func fieldPathsOverlap(a, b []int, exactA, exactB bool) bool {
	if !exactA || !exactB {
		return true
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := range n {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
