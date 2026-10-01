package mir

import (
	"fmt"

	"kigumi/internal/hir"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// Build lowers every body of a checked module through HIR.
func Build(r *sem.Result) *Program {
	p := &Program{R: r, ByEnt: map[sem.EntityID]*Func{}}
	for id := 1; id < len(r.Entities); id++ {
		switch r.Entity(sem.EntityID(id)).Kind {
		case sem.EntFn, sem.EntImplicitMain, sem.EntTest:
			if hf := hir.Lower(r, sem.EntityID(id)); hf != nil {
				p.build(hf)
			}
		}
	}
	p.Entry = p.ByEnt[r.MainFn()]
	return p
}

// builder holds the state of one function under construction.
type builder struct {
	shared map[LocalID]bool
	p      *Program
	r      *sem.Result
	fn     *Func
	cur    BlockID
	locals map[sem.EntityID]LocalID
	scopes []*scope
	loops  []*loopCtx
	ret    sem.TypeID
}

// scope tracks the owned locals and deferred statements of one block.
type scope struct {
	owned  []LocalID
	defers []*hir.Stmt
}

type loopCtx struct {
	cont, exit BlockID
	depth      int
	// valued and result serve a `break` carrying a value (A9-1b): result is
	// the local it assigns before jumping to exit. Local 0 is a valid id,
	// so valued (not a zero check on result) says whether this loop has one.
	valued bool
	result LocalID
}

func (p *Program) build(hf *hir.Func) *Func {
	f := &Func{Name: p.funcName(hf.Ent), Ent: hf.Ent, File: hf.File, Ret: hf.Ret}
	b := &builder{p: p, r: p.R, fn: f, locals: map[sem.EntityID]LocalID{}, shared: map[LocalID]bool{}, ret: hf.Ret}
	b.newBlock()
	b.pushScope()
	if hf.Self != nil {
		// self goes through param(), like any other parameter, so a self
		// captured mutably by a closure gets the same OpNewCell backing.
		f.Params = append(f.Params, b.param(hf.Self))
		self := b.locals[hf.Self.Ent]
		f.Locals[self].Borrowed = !hf.SelfMove
		// A `self`/`mut self` receiver is borrowed from the caller and a
		// destructor's `move self` is being destroyed already: neither is
		// dropped by the method itself.
		if !hf.SelfMove || hf.Destructor {
			b.scopes[0].owned = nil
		}
	}
	for _, d := range hf.Captures {
		id := b.declare(d.Ent)
		f.Params = append(f.Params, id)
		// A closure capturing its enclosing method's `mut self` inherits
		// the same borrowed-place status: EfSelf without EfMove marks
		// mut/plain self on the entity itself, since a closure's own Func
		// has no hf.Self to read it from.
		e := b.r.Entity(d.Ent).Flags
		f.Locals[id].Borrowed = e&sem.EfSelf != 0 && e&sem.EfMove == 0
	}
	for _, decls := range [][]*hir.LocalDecl{hf.Params, hf.Witness, hf.Const} {
		for _, d := range decls {
			f.Params = append(f.Params, b.param(d))
		}
	}
	if hf.Kind == hir.FuncMain {
		for _, s := range hf.Main.Stmts {
			b.stmt(s)
		}
		b.exitScope()
		f.ImplicitOk = true
		b.returnValue(b.unit(hf.Main.Node))
	} else {
		v := b.detach(b.expr(hf.Body))
		b.exitScope()
		b.returnValue(v)
	}
	p.Funcs = append(p.Funcs, f)
	p.ByEnt[hf.Ent] = f
	analyzeOwnership(p, f)
	analyzeBorrows(p, f)
	checkBorrowedMoves(p, f)
	return f
}

func (b *builder) newBlock() BlockID {
	b.fn.Blocks = append(b.fn.Blocks, Block{})
	b.cur = BlockID(len(b.fn.Blocks) - 1)
	return b.cur
}

// newBlockAt creates a block without moving the cursor.
func (b *builder) newBlockAt() BlockID {
	saved := b.cur
	id := b.newBlock()
	b.cur = saved
	return id
}

func (b *builder) block() *Block { return &b.fn.Blocks[b.cur] }

func (b *builder) emit(in Inst) LocalID {
	// A borrow never owns what it aliases, so it has nothing to drop.
	if in.Op == OpDrop && len(in.Args) == 1 && (b.r.Types.Kind(b.fn.Locals[in.Args[0]].Type) == sem.KRef || b.fn.Locals[in.Args[0]].Borrowed) {
		return in.Dst
	}
	if b.block().Term.Op != TermNone {
		b.newBlock()
	}
	b.block().Insts = append(b.block().Insts, in)
	return in.Dst
}

func (b *builder) term(t Term) {
	if b.block().Term.Op == TermNone {
		b.block().Term = t
	}
}

func (b *builder) jump(to BlockID) { b.term(Term{Op: TermJump, Targets: []BlockID{to}}) }

func (b *builder) temp(t sem.TypeID) LocalID {
	b.fn.Locals = append(b.fn.Locals, Local{Name: fmt.Sprintf("t%d", len(b.fn.Locals)), Type: t})
	return LocalID(len(b.fn.Locals) - 1)
}

func (b *builder) unit(n syntax.NodeID) LocalID {
	return b.emit(Inst{Op: OpUnit, Dst: b.temp(sem.TyUnit), Node: n})
}

// never yields a placeholder local after a diverging expression.
func (b *builder) never(n syntax.NodeID) LocalID {
	b.newBlock()
	return b.emit(Inst{Op: OpUnit, Dst: b.temp(sem.TyNever), Node: n})
}

func (b *builder) assign(dst, src LocalID) {
	if b.fn.Locals[src].Type == sem.TyNever {
		return
	}
	b.emit(Inst{Op: OpCopy, Dst: dst, Args: []LocalID{src}})
}

// returnValue ends the function with v; the caller has unwound scopes.
func (b *builder) returnValue(v LocalID) {
	b.term(Term{Op: TermReturn, Args: []LocalID{v}})
}

func (b *builder) variantNamed(adt sem.EntityID, name string) sem.EntityID {
	for _, v := range b.r.TypeDecl(adt).Variants {
		if b.r.Entity(v).Name == name {
			return v
		}
	}
	return 0
}

// closureOrdinal numbers a closure among its parent's closures in
// declaration order, so names stay stable across unrelated edits and std
// load order (a global entity id would not).
func (p *Program) closureOrdinal(ent sem.EntityID) int {
	parent := p.R.Entity(ent).Parent
	n := 0
	for id := sem.EntityID(1); id <= ent; id++ {
		if e := p.R.Entity(id); e.Kind == sem.EntClosure && e.Parent == parent {
			n++
		}
	}
	return n
}
