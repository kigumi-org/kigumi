package mir

import (
	"kigumi/internal/diag"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// checkBorrowedMoves rejects moving a borrowed value out of the function
// that borrows it: the caller keeps its value, so handing it on would alias
// two owners. A borrow is a `self` receiver or a `&T` local, and anything
// projected out of one (a field, an element, a payload) stays borrowed.
func checkBorrowedMoves(p *Program, f *Func) {
	borrowed := make([]bool, len(f.Locals))
	defNode := make([]syntax.NodeID, len(f.Locals))
	for i, l := range f.Locals {
		borrowed[i] = l.Borrowed || p.R.Types.Kind(l.Type) == sem.KRef
	}
	isClosure := p.R.Entity(f.Ent).Kind == sem.EntClosure
	if isClosure {
		for i, cap := range p.R.Closure(f.Ent).Captures {
			if cap.Mode == sem.CapBorrow {
				borrowed[f.Params[i]] = true
			}
		}
	}
	for _, blk := range f.Blocks {
		for _, in := range blk.Insts {
			if in.Dst != 0 {
				defNode[in.Dst] = in.Node
			}
			propagates := len(in.Args) > 0 && borrowed[in.Args[0]] && !in.Share && f.Locals[in.Dst].Ent == 0
			switch in.Op {
			case OpField, OpIndex, OpPayload, OpUnbox, OpCopy:
				if propagates {
					borrowed[in.Dst] = true
				}
			// A field taken out of a `self`/`mut self` receiver is meant to
			// escape: it stays untainted for the receiver's
			// own method. A closure only lends a captured self, so the same
			// move through its own cell (OpCellGet) stays tainted there.
			case OpFieldMove, OpCellGet:
				if propagates && isClosure {
					borrowed[in.Dst] = true
				}
			}
		}
	}
	reported := map[syntax.NodeID]bool{}
	report := func(n syntax.NodeID, l LocalID) {
		if n == 0 || reported[n] {
			return
		}
		reported[n] = true
		t := p.R.Tree(f.File)
		msg := "a value borrowed here cannot be moved out; clone it"
		if name := f.Locals[l].Name; f.Locals[l].Ent != 0 {
			msg = "`" + name + "` is borrowed by this method and cannot be moved out; clone it or take `move self`"
			if p.R.Entity(f.Ent).Kind == sem.EntClosure {
				msg = "`" + name + "` is lent to this closure and cannot be moved out; clone it"
			}
		}
		f.Diags = append(f.Diags, diag.Diagnostic{Severity: diag.Error, Loc: diag.At(t.File, t.Span(n)), Code: "move-from-borrow", Msg: msg})
	}
	mutRef := func(l LocalID) bool {
		t := f.Locals[l].Type
		return p.R.Types.Kind(t) == sem.KRef && !p.R.IsCopy(t)
	}
	// checkAliasedMutArgs rejects a reborrowed `&mut T` (own_borrowed no
	// longer moves it) handed to the same call twice: the callee
	// would see two exclusive borrows of the one referent.
	checkAliasedMutArgs := func(n syntax.NodeID, args []LocalID) {
		if n == 0 || reported[n] {
			return
		}
		seen := map[LocalID]bool{}
		for _, a := range args {
			if !mutRef(a) {
				continue
			}
			if seen[a] {
				reported[n] = true
				t := p.R.Tree(f.File)
				f.Diags = append(f.Diags, diag.Diagnostic{Severity: diag.Error, Loc: diag.At(t.File, t.Span(n)), Code: "borrow-conflict",
					Msg: "`" + f.Locals[a].Name + "` is passed twice as `&mut`; the callee would get two exclusive borrows of the same value"})
				return
			}
			seen[a] = true
		}
	}
	moveOnly := func(l LocalID) bool {
		t := f.Locals[l].Type
		return borrowed[l] && !p.R.IsCopy(t) && p.R.Types.Kind(t) != sem.KRef
	}
	for _, blk := range f.Blocks {
		for _, in := range blk.Insts {
			switch in.Op {
			case OpMove:
				if borrowed[in.Args[0]] {
					report(in.Node, in.Args[0])
				}
			case OpCopy:
				if f.Locals[in.Dst].Ent != 0 && !in.Share && moveOnly(in.Args[0]) {
					report(in.Node, in.Args[0])
				}
			case OpSetField, OpSetIndex, OpNewCell, OpBoxReplace:
				if v := in.Args[len(in.Args)-1]; moveOnly(v) {
					report(in.Node, v)
				}
			case OpCall, OpCallValue, OpBuiltin:
				for i, a := range in.Args {
					if moveOnly(a) && p.consumesArg(&in, i) {
						report(in.Node, a)
					}
				}
				checkAliasedMutArgs(in.Node, in.Args)
			case OpRecord, OpVariant:
				for _, a := range in.Args {
					// A spread's projections were already reported at the spread itself.
					if moveOnly(a) && !reported[defNode[a]] {
						report(in.Node, a)
					}
				}
			}
		}
		if blk.Term.Op == TermReturn {
			for _, a := range blk.Term.Args {
				if moveOnly(a) {
					report(defNode[a], a)
				}
			}
		}
	}
}
