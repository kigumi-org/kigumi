package mir

import (
	"kigumi/internal/hir"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

func (b *builder) matchExpr(e *hir.Expr) LocalID {
	scrutinee := b.expr(e.Args[0])
	// The scrutinee's scope opens after it is assigned, so every exit of
	// the arms (including early ones) drops a live temporary.
	b.pushScope()
	b.ownTemp(scrutinee)
	result := b.temp(e.Type)
	join := b.newBlockAt()
	for _, arm := range e.Arms {
		body, next := b.newBlockAt(), b.newBlockAt()
		cond := b.testPattern(arm.Pat, scrutinee)
		b.term(Term{Op: TermBranch, Args: []LocalID{cond}, Targets: []BlockID{body, next}})
		b.cur = body
		b.pushScope()
		b.bindPattern(arm.Pat, scrutinee)
		if arm.Guard != nil {
			gv := b.expr(arm.Guard)
			run, fail := b.newBlockAt(), b.newBlockAt()
			b.term(Term{Op: TermBranch, Args: []LocalID{gv}, Targets: []BlockID{run, fail}})
			// A failed guard leaves the arm: its bindings die here too.
			b.cur = fail
			b.unwindScope(b.scopes[len(b.scopes)-1], false)
			b.jump(next)
			b.cur = run
		}
		b.assign(result, b.expr(arm.Body))
		b.exitScope()
		b.jump(join)
		b.cur = next
	}
	b.emit(Inst{Op: OpPanic, Str: "no match arm matched", Node: e.Node})
	b.term(Term{Op: TermUnreachable})
	b.cur = join
	b.exitScope()
	return result
}

// testPattern yields a Bool local: does v match p.
func (b *builder) testPattern(p *hir.Pat, v LocalID) LocalID {
	switch p.Kind {
	case hir.PatWild, hir.PatBind:
		return b.emit(Inst{Op: OpConst, Dst: b.temp(sem.TyBool), Lit: sem.Literal{Kind: sem.LitBool, Bool: true}, Type: sem.TyBool, Node: p.Node})
	case hir.PatLit:
		lit := b.expr(p.Lit)
		return b.emit(Inst{Op: OpBinary, Dst: b.temp(sem.TyBool), Str: "==", Args: []LocalID{v, lit}, Node: p.Node})
	case hir.PatCtor:
		return b.testCtor(p, v)
	case hir.PatRecord:
		return b.testRecord(p, v)
	case hir.PatOr:
		cond := b.testPattern(p.Subs[0], v)
		for _, alt := range p.Subs[1:] {
			cond = b.orElse(cond, func() LocalID { return b.testPattern(alt, v) })
		}
		return cond
	case hir.PatRange:
		return b.testRange(p, v)
	}
	return b.emit(Inst{Op: OpConst, Dst: b.temp(sem.TyBool), Lit: sem.Literal{Kind: sem.LitBool}, Type: sem.TyBool, Node: p.Node})
}

// testRange yields lo <= v (< or <=) hi, `v`'s type throughout.
func (b *builder) testRange(p *hir.Pat, v LocalID) LocalID {
	lo := b.emit(Inst{Op: OpBinary, Dst: b.temp(sem.TyBool), Str: ">=", Args: []LocalID{v, b.expr(p.Lo)}, Node: p.Node})
	hiOp := "<"
	if p.Inclusive {
		hiOp = "<="
	}
	return b.andThen(lo, func() LocalID {
		return b.emit(Inst{Op: OpBinary, Dst: b.temp(sem.TyBool), Str: hiOp, Args: []LocalID{v, b.expr(p.Hi)}, Node: p.Node})
	})
}

func (b *builder) testCtor(p *hir.Pat, v LocalID) LocalID {
	if p.IsType {
		cond := b.emit(Inst{Op: OpIsType, Dst: b.temp(sem.TyBool), Ent: p.Ent, Args: []LocalID{v}, Node: p.Node})
		if len(p.Subs) == 1 {
			return b.andThen(cond, func() LocalID {
				inner := b.emit(Inst{Op: OpUnbox, Dst: b.temp(p.Subs[0].Type), Args: []LocalID{v}, Node: p.Node})
				return b.testPattern(p.Subs[0], inner)
			})
		}
		return cond
	}
	cond := b.emit(Inst{Op: OpIsVariant, Dst: b.temp(sem.TyBool), Ent: p.Ent, Args: []LocalID{v}, Node: p.Node})
	for i, s := range p.Subs {
		if s.Kind == hir.PatWild || s.Kind == hir.PatBind {
			continue
		}
		cond = b.andThen(cond, func() LocalID {
			payload := b.emit(Inst{Op: OpPayload, Dst: b.temp(s.Type), Args: []LocalID{v}, Index: i, Node: s.Node})
			return b.testPattern(s, payload)
		})
	}
	return cond
}

func (b *builder) testRecord(p *hir.Pat, v LocalID) LocalID {
	var cond LocalID
	if p.Variant {
		cond = b.emit(Inst{Op: OpIsVariant, Dst: b.temp(sem.TyBool), Ent: p.Ent, Args: []LocalID{v}, Node: p.Node})
	} else {
		cond = b.emit(Inst{Op: OpConst, Dst: b.temp(sem.TyBool), Lit: sem.Literal{Kind: sem.LitBool, Bool: true}, Type: sem.TyBool, Node: p.Node})
	}
	for _, f := range p.Fields {
		if f.Sub == nil || f.Sub.Kind == hir.PatWild || f.Sub.Kind == hir.PatBind {
			continue
		}
		cond = b.andThen(cond, func() LocalID {
			return b.testPattern(f.Sub, b.fieldOf(f, v))
		})
	}
	return cond
}

// fieldOf reads the sub-value a record-pattern field names.
func (b *builder) fieldOf(f *hir.PatField, v LocalID) LocalID {
	if f.Payload {
		return b.emit(Inst{Op: OpPayload, Dst: b.temp(f.Type), Args: []LocalID{v}, Index: f.Index, Node: f.Node})
	}
	return b.emit(Inst{Op: OpField, Dst: b.temp(f.Type), Args: []LocalID{v}, Index: f.Index, Node: f.Node})
}

// bindPattern declares the bindings of a pattern that already matched.
func (b *builder) bindPattern(p *hir.Pat, v LocalID) {
	switch p.Kind {
	case hir.PatBind:
		id := b.declare(p.Ent)
		if b.fn.Locals[id].Cell {
			if b.shared[v] {
				v = b.emit(Inst{Op: OpCopy, Dst: b.temp(b.fn.Locals[v].Type), Args: []LocalID{v}, Node: p.Node, Share: true})
			}
			b.emit(Inst{Op: OpNewCell, Dst: id, Args: []LocalID{v}, Node: p.Node})
			return
		}
		b.bindLocal(id, v, p.Node)
	case hir.PatCtor:
		if p.IsType {
			if len(p.Subs) == 1 {
				b.bindPattern(p.Subs[0], b.emit(Inst{Op: OpUnbox, Dst: b.temp(p.Subs[0].Type), Args: []LocalID{v}, Node: p.Node}))
			}
			return
		}
		for i, s := range p.Subs {
			if s.Kind == hir.PatWild {
				continue
			}
			b.bindPattern(s, b.emit(Inst{Op: OpPayload, Dst: b.temp(s.Type), Args: []LocalID{v}, Index: i, Node: s.Node}))
		}
	case hir.PatRecord:
		for _, f := range p.Fields {
			val := b.fieldOf(f, v)
			if f.Sub != nil {
				b.bindPattern(f.Sub, val)
				continue
			}
			b.bindLocal(b.declare(f.Bind), val, f.Node)
		}
	}
}

// bindLocal initializes a pattern binding: a `&mut ScalarT` binding of a
// plain ScalarT payload gets a cell, since storePlace's OpCellSet
// trusts this static shape and a bare OpBorrow alias would hand it a
// non-cell object; a `&U` binding of a U payload otherwise aliases it, and
// anything else copies.
func (b *builder) bindLocal(id, v LocalID, n syntax.NodeID) {
	tt := b.r.Types
	idT := b.fn.Locals[id].Type
	vNotRef := tt.Kind(b.fn.Locals[v].Type) != sem.KRef
	if _, ok := b.r.ScalarRefElem(idT); ok && vNotRef {
		// v's own type is the unwrapped payload (binding mode strips
		// the pattern's own type to the scrutinee), so it needs the same
		// OpBorrow retype + OpShare retain as an explicit `&mut expr` of a
		// place (build_expr.go's hir.Borrow) before OpNewCell can take it:
		// OpNewCell requires its argument's static type to already match id's.
		ref := b.emit(Inst{Op: OpBorrow, Dst: b.temp(idT), Args: []LocalID{v}, Index: 1, Node: n})
		owned := b.emit(Inst{Op: OpShare, Dst: b.temp(idT), Args: []LocalID{ref}, Node: n, Copy: true})
		b.emit(Inst{Op: OpNewCell, Dst: id, Args: []LocalID{owned}, Node: n})
		return
	}
	if tt.Kind(idT) == sem.KRef && vNotRef {
		mut := 0
		if tt.RefMut(idT) {
			mut = 1
		}
		b.emit(Inst{Op: OpBorrow, Dst: id, Args: []LocalID{v}, Index: mut, Node: n})
		return
	}
	b.emit(Inst{Op: OpCopy, Dst: id, Args: []LocalID{v}, Node: n, Share: b.shared[v]})
}
