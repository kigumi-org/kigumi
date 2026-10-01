package mir

import "kigumi/internal/sem"

// verifyDefs checks definition before use. A temporary must be assigned
// on every path to each of its uses; a named local must be assigned on at
// least one, because the checker narrows the reads of a pattern binding
// behind `&&` to the matched path, which the CFG cannot see.
func verifyDefs(f *Func) *Error {
	n := len(f.Locals)
	must := make([][]bool, len(f.Blocks))
	may := make([][]bool, len(f.Blocks))
	entryMust, entryMay := make([]bool, n), make([]bool, n)
	for _, prm := range f.Params {
		entryMust[prm], entryMay[prm] = true, true
	}
	must[0], may[0] = entryMust, entryMay
	seen := make([]bool, len(f.Blocks))
	work := []BlockID{0}
	for len(work) > 0 {
		bid := work[0]
		work = work[1:]
		outMust, outMay := transferDefs(f, bid, must[bid], may[bid])
		if diverges(f, &f.Blocks[bid]) {
			continue
		}
		for _, succ := range f.Blocks[bid].Term.Targets {
			if !seen[succ] {
				seen[succ] = true
				must[succ], may[succ] = append([]bool{}, outMust...), append([]bool{}, outMay...)
				work = append(work, succ)
				continue
			}
			changed := false
			for l := 0; l < n; l++ {
				if must[succ][l] && !outMust[l] {
					must[succ][l], changed = false, true
				}
				if !may[succ][l] && outMay[l] {
					may[succ][l], changed = true, true
				}
			}
			if changed {
				work = append(work, succ)
			}
		}
	}
	seen[0] = true
	for bid := range f.Blocks {
		if !seen[bid] {
			continue
		}
		if err := checkDefs(f, BlockID(bid), must[bid], may[bid]); err != nil {
			return err
		}
	}
	return nil
}

// diverges reports a block that never reaches its terminator: a panic or
// a call that yields Never ends the function, so the builder's jump after
// it is dead and the block defines nothing for its successors.
func diverges(f *Func, blk *Block) bool {
	for _, in := range blk.Insts {
		if in.Op == OpPanic || (in.Op == OpBuiltin && in.Str == "panic") {
			return true
		}
		if in.Op.Yields() && in.Op != OpUnit && f.Locals[in.Dst].Type == sem.TyNever {
			return true
		}
	}
	return false
}

func transferDefs(f *Func, bid BlockID, inMust, inMay []bool) ([]bool, []bool) {
	must, may := append([]bool{}, inMust...), append([]bool{}, inMay...)
	for _, in := range f.Blocks[bid].Insts {
		if in.Op.Yields() {
			must[in.Dst], may[in.Dst] = true, true
		}
	}
	return must, may
}

func checkDefs(f *Func, bid BlockID, inMust, inMay []bool) *Error {
	must, may := append([]bool{}, inMust...), append([]bool{}, inMay...)
	defined := func(l LocalID) bool {
		if f.Locals[l].Ent != 0 {
			return may[l]
		}
		return must[l]
	}
	blk := &f.Blocks[bid]
	for i, in := range blk.Insts {
		for _, a := range in.Args {
			if !defined(a) {
				return fail("defs", bid, i, "%s reads %%%s before any definition", in.Op, f.Locals[a].Name)
			}
		}
		if in.Op.Yields() {
			must[in.Dst], may[in.Dst] = true, true
		}
	}
	for _, a := range blk.Term.Args {
		if !defined(a) {
			return fail("defs", bid, len(blk.Insts), "terminator reads %%%s before any definition", f.Locals[a].Name)
		}
	}
	return nil
}
