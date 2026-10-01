package mir

func uses(in Inst) []LocalID { return in.Args }

// liveness computes, per block, the borrow holders live at exit.
func (c *borrowCheck) liveness() {
	n := len(c.f.Blocks)
	c.liveOut = make([]map[LocalID]bool, n)
	liveIn := make([]map[LocalID]bool, n)
	for i := range n {
		c.liveOut[i] = map[LocalID]bool{}
		liveIn[i] = map[LocalID]bool{}
	}
	for changed := true; changed; {
		changed = false
		for bid := n - 1; bid >= 0; bid-- {
			blk := &c.f.Blocks[bid]
			out := map[LocalID]bool{}
			for _, t := range blk.Term.Targets {
				for l := range liveIn[t] {
					out[l] = true
				}
			}
			live := map[LocalID]bool{}
			for l := range out {
				live[l] = true
			}
			for _, a := range blk.Term.Args {
				if _, ok := c.holders[a]; ok {
					live[a] = true
				}
			}
			for i := len(blk.Insts) - 1; i >= 0; i-- {
				in := blk.Insts[i]
				if _, ok := c.holders[in.Dst]; ok && in.Op != OpDrop && in.Op != OpSetField && in.Op != OpSetIndex && in.Op != OpCellSet && in.Op != OpBoxReplace {
					delete(live, in.Dst)
				}
				for _, a := range uses(in) {
					if _, ok := c.holders[a]; ok {
						live[a] = true
					}
				}
			}
			if !sameSet(out, c.liveOut[bid]) || !sameSet(live, liveIn[bid]) {
				changed = true
			}
			c.liveOut[bid] = out
			liveIn[bid] = live
		}
	}
}

func sameSet(a, b map[LocalID]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// definitions computes, per block, the holders assigned on some path into it.
func (c *borrowCheck) definitions() {
	n := len(c.f.Blocks)
	c.definedIn = make([]map[LocalID]bool, n)
	for i := range n {
		c.definedIn[i] = map[LocalID]bool{}
	}
	for changed := true; changed; {
		changed = false
		for bid := range n {
			out := map[LocalID]bool{}
			for l := range c.definedIn[bid] {
				out[l] = true
			}
			for _, in := range c.f.Blocks[bid].Insts {
				if _, ok := c.holders[in.Dst]; ok && shapes[in.Op].result {
					out[in.Dst] = true
				}
			}
			for _, t := range c.f.Blocks[bid].Term.Targets {
				for l := range out {
					if !c.definedIn[t][l] {
						c.definedIn[t][l] = true
						changed = true
					}
				}
			}
		}
	}
}

// definedBefore lists, per instruction of a block, the holders assigned
// before it on some path.
func (c *borrowCheck) definedBefore(bid BlockID) []map[LocalID]bool {
	blk := &c.f.Blocks[bid]
	cur := map[LocalID]bool{}
	for l := range c.definedIn[bid] {
		cur[l] = true
	}
	out := make([]map[LocalID]bool, len(blk.Insts))
	for i, in := range blk.Insts {
		out[i] = cur
		if _, ok := c.holders[in.Dst]; ok && shapes[in.Op].result {
			next := map[LocalID]bool{}
			for l := range cur {
				next[l] = true
			}
			next[in.Dst] = true
			cur = next
		}
	}
	return out
}

func intersect(a, b map[LocalID]bool) map[LocalID]bool {
	out := map[LocalID]bool{}
	for l := range a {
		if b[l] {
			out[l] = true
		}
	}
	return out
}
