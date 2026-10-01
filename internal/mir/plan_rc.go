package mir

import (
	"fmt"

	"kigumi/internal/sem"
)

// PlanRC rewrites every OpCopy into an explicit OpAlias or OpShare and
// makes every other copy/retain/release decision explicit too, so a
// backend afterward translates opcodes one by one without consulting Plan,
// copyable, inPlace or mustCopy.
func PlanRC(p *Program) error {
	for _, f := range p.allFuncs() {
		p.planRCFunc(f)
	}
	return nil
}

// rcState is the working memory PlanRC threads across one function's
// blocks, in the same block-array order the two backends walk today: which
// temporary stands for which named local (alias), and which temporaries a
// borrow produced (never copied, never re-aliased further).
type rcState struct {
	alias    map[LocalID]LocalID
	borrowed map[LocalID]bool
}

func (s *rcState) origin(l LocalID) LocalID {
	for {
		src, ok := s.alias[l]
		if !ok {
			return l
		}
		l = src
	}
}

func (p *Program) planRCFunc(f *Func) {
	plan := p.planFunc(f)
	st := &rcState{alias: map[LocalID]LocalID{}, borrowed: map[LocalID]bool{}}
	for bid := range f.Blocks {
		p.planRCBlock(f, bid, plan, st)
	}
}

func (p *Program) planRCBlock(f *Func, bid int, plan *Plan, st *rcState) {
	blk := &f.Blocks[bid]
	orig := blk.Insts
	out := make([]Inst, 0, len(orig))
	for k := range orig {
		in := orig[k]
		p.shareArgs(f, &in, st, &out)
		p.rewriteCopy(f, &in, plan, st)
		out = append(out, in)
		for _, t := range plan.Releases[Point{bid, k}] {
			out = append(out, Inst{Op: OpRelease, Args: []LocalID{t}})
		}
	}
	p.shareReturn(f, &blk.Term, st, &out)
	for _, t := range plan.Releases[Point{bid, len(orig)}] {
		out = append(out, Inst{Op: OpRelease, Args: []LocalID{t}})
	}
	blk.Insts = out
}

// shareArgs inserts each argument's extra reference right before the
// instruction that reads it, instead of the backends inserting it inline
// themselves.
func (p *Program) shareArgs(f *Func, in *Inst, st *rcState, out *[]Inst) {
	if len(in.Args) == 0 {
		return
	}
	args := append([]LocalID(nil), in.Args...)
	for i, a := range args {
		if p.inPlace(*in, i) || st.borrowed[a] || !p.mustCopy(f, *in, st.origin(a)) {
			continue
		}
		src := st.origin(a)
		t := f.newTemp(f.Locals[a].Type)
		*out = append(*out, Inst{Op: OpShare, Dst: t, Args: []LocalID{a}, Copy: p.copyable(f.Locals[src].Type)})
		args[i] = t
	}
	in.Args = args
}

// rewriteCopy turns one OpCopy into OpAlias (no runtime call) or OpShare
// (rt_copy/rt_retain), following the same cases the backends' OpCopy
// handling does: an eager Share, a source that is itself a temporary or a
// borrow (a pure alias), a single-definition temporary destination
// deferred to its own consuming use, or an eager share by default.
func (p *Program) rewriteCopy(f *Func, in *Inst, plan *Plan, st *rcState) {
	switch in.Op {
	case OpBorrow:
		st.borrowed[in.Dst] = true
		return
	case OpCopy:
	default:
		return
	}
	src := st.origin(in.Args[0])
	switch {
	case in.Share:
		in.Op, in.Copy = OpShare, p.copyable(f.Locals[src].Type)
	case f.Locals[src].Ent == 0 || st.borrowed[in.Args[0]]:
		in.Op = OpAlias
	case f.Locals[in.Dst].Ent == 0 && plan.Defs[in.Dst] <= 1 && !plan.Materialize[in.Dst]:
		in.Op = OpAlias
		st.alias[in.Dst] = src
	default:
		in.Op, in.Copy = OpShare, p.copyable(f.Locals[src].Type)
	}
	in.Share = false
}

// shareReturn mirrors the backends' TermReturn case: returning a named
// local the scope still owns hands the caller its own reference.
func (p *Program) shareReturn(f *Func, t *Term, st *rcState, out *[]Inst) {
	if t.Op != TermReturn {
		return
	}
	a := t.Args[0]
	src := st.origin(a)
	if st.borrowed[a] || f.Locals[src].Ent == 0 {
		return
	}
	nt := f.newTemp(f.Locals[a].Type)
	*out = append(*out, Inst{Op: OpShare, Dst: nt, Args: []LocalID{a}, Copy: p.copyable(f.Locals[src].Type)})
	t.Args = []LocalID{nt}
}

// newTemp appends a fresh temporary of type t, named like the builder's
// own temporaries so a golden dump reads the same way.
func (f *Func) newTemp(t sem.TypeID) LocalID {
	f.Locals = append(f.Locals, Local{Name: fmt.Sprintf("t%d", len(f.Locals)), Type: t})
	return LocalID(len(f.Locals) - 1)
}
