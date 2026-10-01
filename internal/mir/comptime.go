package mir

import "kigumi/internal/sem"

// ComptimeFunc is one `comptime { ... }` block: its body as a function, and
// the placeholder instruction in Owner that Fold turns into the constant.
type ComptimeFunc struct {
	Site  sem.ComptimeSite
	Func  *Func
	Owner *Func
	Block BlockID
	// Slot is the placeholder's Inst.Index (Op: OpBuiltin, Str: "comptime"):
	// its identity, since PlanRC rebuilds Owner.Blocks[Block].Insts before
	// Fold runs and can shift a plain slice index recorded at build time.
	Slot int
	// Type is the block's type at the use site, which the constant takes.
	Type sem.TypeID
}

// Fold records the evaluated value with the checker and replaces the
// block's placeholder with a constant, so every back end sees a literal.
func (p *Program) Fold(cf *ComptimeFunc, lit sem.Literal) {
	p.R.SetLiteral(cf.Site.File, cf.Site.Node, lit)
	insts := cf.Owner.Blocks[cf.Block].Insts
	for i := range insts {
		in := &insts[i]
		if in.Op == OpBuiltin && in.Str == "comptime" && in.Index == cf.Slot {
			*in = Inst{Op: OpConst, Dst: in.Dst, Lit: lit, Type: cf.Type, Node: in.Node}
			return
		}
	}
	panic("mir: Fold: placeholder not found")
}
