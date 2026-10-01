package mir

import (
	"kigumi/internal/hir"
	"kigumi/internal/sem"
)

// asm lowers an asm block to one OpAsm over its input values (in and inout
// operands in order); the backend reads the rest from sem.AsmInfo.
func (b *builder) asm(e *hir.Expr) LocalID {
	var args []LocalID
	for _, a := range e.Args {
		args = append(args, b.expr(a))
	}
	dst := b.emit(Inst{Op: OpAsm, Dst: b.temp(e.Type), Type: e.Type, Args: args, Node: e.Node})
	if e.Type == sem.TyNever {
		b.term(Term{Op: TermUnreachable})
		return b.never(e.Node)
	}
	return dst
}
