package mir

import (
	"kigumi/internal/hir"
	"kigumi/internal/sem"
)

func (b *builder) stmt(s *hir.Stmt) {
	switch s.Kind {
	case hir.LetStmt:
		b.letStmt(s)
	case hir.AssignStmt:
		b.assignStmt(s)
	case hir.DiscardStmt:
		b.discard(s)
	case hir.ExprStmt:
		v := b.expr(s.Expr)
		if b.r.Types.Kind(s.Expr.Type) != sem.KRef {
			b.emit(Inst{Op: OpDrop, Args: []LocalID{v}, Node: s.Node})
		}
	case hir.DeferStmt:
		sc := b.scopes[len(b.scopes)-1]
		sc.defers = append(sc.defers, s)
	case hir.EvalStmt:
		b.expr(s.Expr)
	}
}

func (b *builder) letStmt(s *hir.Stmt) {
	var v LocalID
	if s.Pat.Borrow {
		v = b.expr(s.Expr)
		b.ownTemp(v)
	} else {
		v = b.exprMoved(s.Expr)
	}
	if s.Else == nil {
		b.bindPattern(s.Pat, v)
		return
	}
	okBlock, elseBlock := b.newBlock(), b.newBlock()
	entry := BlockID(len(b.fn.Blocks) - 3)
	b.cur = entry
	cond := b.testPattern(s.Pat, v)
	b.term(Term{Op: TermBranch, Args: []LocalID{cond}, Targets: []BlockID{okBlock, elseBlock}})
	b.cur = elseBlock
	b.blockExpr(s.Else)
	b.term(Term{Op: TermUnreachable})
	b.cur = okBlock
	b.bindPattern(s.Pat, v)
}

// exprMoved lowers an expression whose value leaves the current scope: a
// named local is copied or moved into a temporary before any coercion
// wraps it, so the scope's drop of the local stays balanced.
func (b *builder) exprMoved(e *hir.Expr) LocalID {
	v := b.transfer(e, b.exprRaw(e))
	if e.Kind == hir.Fallback {
		return v
	}
	return b.coerce(e.Coercion, e.Node, v)
}

// transfer moves a move-only source local or copies a structural value.
func (b *builder) transfer(src *hir.Expr, v LocalID) LocalID {
	if src.Kind != hir.Local {
		return v
	}
	if _, isLocal := b.locals[src.Ent]; src.Ent == 0 || !isLocal {
		return v
	}
	// A named local is dropped when its scope unwinds, so what leaves the
	// scope travels in a temporary: a copy of a Copy value, a move otherwise.
	t := src.Type
	if b.r.IsCopy(t) {
		return b.emit(Inst{Op: OpCopy, Dst: b.temp(t), Args: []LocalID{v}, Node: src.Node})
	}
	return b.emit(Inst{Op: OpMove, Dst: b.temp(t), Args: []LocalID{v}, Node: src.Node})
}

// discard lowers `_ = value`. Discarding a binding changes nothing unless
// it is move-only: the binding keeps a borrowed or Copy value and drops it
// at scope exit, while a move-only value is moved out and dropped here.
func (b *builder) discard(s *hir.Stmt) {
	rhs := s.Expr
	if l, ok := b.locals[rhs.Ent]; ok && rhs.Kind == hir.Local {
		lt := b.fn.Locals[l].Type
		if b.fn.Locals[l].Borrowed || b.r.IsCopy(lt) {
			return
		}
	}
	v := b.exprMoved(rhs)
	b.emit(Inst{Op: OpDrop, Args: []LocalID{v}, Node: s.Node})
}

func (b *builder) assignStmt(s *hir.Stmt) {
	p := b.resolvePlace(s.Place)
	if s.Compound {
		cur := b.loadPlace(p)
		r := b.expr(s.Expr)
		var out LocalID
		if s.OpEnt != 0 {
			out = b.emit(Inst{Op: OpCall, Dst: b.temp(s.Place.Type), Ent: s.OpEnt, Args: []LocalID{cur, r}, Node: s.Node})
		} else {
			out = b.emit(Inst{Op: OpBinary, Dst: b.temp(s.Place.Type), Str: s.OpText, Args: []LocalID{cur, r}, Node: s.Node})
		}
		b.storePlace(p, out)
		return
	}
	b.storePlace(p, b.exprMoved(s.Expr))
}
