package mir

// planAliases finds the alias temporaries (single-definition copies of a
// named local) that are still read after the local itself, or another
// alias of it, has been dropped: those must take their own reference when
// they are created instead of standing for the local.
func (p *Program) planAliases(f *Func, isTemp func(LocalID) bool, defs map[LocalID]int) map[LocalID]bool {
	origin := map[LocalID]LocalID{}
	defBlock := map[LocalID]int{}
	defIdx := map[LocalID]int{}
	for b, blk := range f.Blocks {
		for k, in := range blk.Insts {
			if in.Op != OpCopy || !isTemp(in.Dst) || len(in.Args) != 1 {
				continue
			}
			src := in.Args[0]
			if o, ok := origin[src]; ok {
				src = o
			}
			if isTemp(src) {
				continue
			}
			origin[in.Dst] = src
			defBlock[in.Dst] = b
			defIdx[in.Dst] = k
		}
	}
	out := map[LocalID]bool{}
	for t, o := range origin {
		if defs[t] != 1 {
			continue
		}
		dropped := false
		for b, blk := range f.Blocks {
			for k, in := range blk.Insts {
				if in.Op == OpDrop && len(in.Args) == 1 && (in.Args[0] == o || origin[in.Args[0]] == o) && (b != defBlock[t] || k > defIdx[t]) {
					dropped = true
				}
			}
		}
		if !dropped {
			continue
		}
		for b, blk := range f.Blocks {
			uses := func(args []LocalID, k int) {
				for _, a := range args {
					if a != t {
						continue
					}
					if b != defBlock[t] || dropAfterDef(f, blk, defIdx[t], k, o, origin) {
						out[t] = true
					}
				}
			}
			for k, in := range blk.Insts {
				uses(in.Args, k)
			}
			uses(blk.Term.Args, len(blk.Insts))
		}
	}
	return out
}

// dropAfterDef reports whether a drop of local o (or of an alias of it)
// sits between the definition at def and the use at use within blk.
func dropAfterDef(f *Func, blk Block, def, use int, o LocalID, origin map[LocalID]LocalID) bool {
	for k := def + 1; k < use && k < len(blk.Insts); k++ {
		in := blk.Insts[k]
		if in.Op == OpDrop && len(in.Args) == 1 && (in.Args[0] == o || origin[in.Args[0]] == o) {
			return true
		}
	}
	return false
}
