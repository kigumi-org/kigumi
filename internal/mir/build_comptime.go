package mir

import (
	"fmt"

	"kigumi/internal/hir"
	"kigumi/internal/sem"
)

// comptime lowers a block's body as a function of its own and leaves a
// placeholder in the enclosing body; the driver evaluates the function in
// the VM's sandbox and folds the result in with Program.Fold.
func (b *builder) comptime(e *hir.Expr) LocalID {
	inner := e.Args[0]
	ret := inner.Type
	if b.r.Types.Kind(ret) == sem.KUntyped {
		ret = sem.TyI64
		if inner.Type == sem.TyUntypedFloat {
			ret = sem.TyF64
		}
	}
	f := &Func{Name: fmt.Sprintf("%s.comptime%d", b.fn.Name, len(b.p.Comptime)), File: b.fn.File, Ret: ret}
	cb := &builder{p: b.p, r: b.r, fn: f, locals: map[sem.EntityID]LocalID{}, shared: map[LocalID]bool{}, ret: ret}
	cb.newBlock()
	cb.pushScope()
	v := cb.detach(cb.expr(inner))
	cb.exitScope()
	cb.returnValue(v)
	analyzeOwnership(b.p, f)
	analyzeBorrows(b.p, f)
	checkBorrowedMoves(b.p, f)
	// Slot tags the placeholder so Fold can find it later by identity: a
	// pass between the builder and fold-comptime (PlanRC) rebuilds each
	// block's Insts and shifts any plain index recorded here.
	slot := len(b.p.Comptime)
	dst := b.emit(Inst{Op: OpBuiltin, Dst: b.temp(e.Type), Str: "comptime", Index: slot, Node: e.Node})
	b.p.Comptime = append(b.p.Comptime, &ComptimeFunc{
		Site:  sem.ComptimeSite{File: b.fn.File, Node: e.Node, Fn: b.fn.Ent},
		Func:  f,
		Owner: b.fn,
		Block: b.cur,
		Slot:  slot,
		Type:  e.Type,
	})
	return dst
}
