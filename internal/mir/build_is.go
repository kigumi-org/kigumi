package mir

import "kigumi/internal/hir"

// ownTemp hands a scrutinee temporary to the innermost scope, which must
// open after the temporary is assigned: pattern bindings copy out of it,
// so it is released with that scope on every exit.
func (b *builder) ownTemp(v LocalID) {
	if b.fn.Locals[v].Ent == 0 {
		b.own(v)
		b.shared[v] = true
	}
}

// isExpr lowers `v is pat`: the test, then the bindings on success. A
// temporary scrutinee dies at the join, after the bindings copied out of
// it; an `is` may sit behind `&&`, so its cleanup stays inside its own
// evaluation rather than in the enclosing scope.
func (b *builder) isExpr(p *hir.Pat, v LocalID) LocalID {
	cond := b.testPattern(p, v)
	yes, join := b.newBlockAt(), b.newBlockAt()
	b.term(Term{Op: TermBranch, Args: []LocalID{cond}, Targets: []BlockID{yes, join}})
	b.cur = yes
	if b.fn.Locals[v].Ent == 0 {
		b.shared[v] = true
	}
	b.bindPattern(p, v)
	b.jump(join)
	b.cur = join
	if b.fn.Locals[v].Ent == 0 {
		b.emit(Inst{Op: OpDrop, Args: []LocalID{v}, Node: p.Node})
	}
	return cond
}
