package mir

import (
	"kigumi/internal/hir"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

func (b *builder) shortAnd(e *hir.Expr) LocalID {
	result := b.temp(sem.TyBool)
	l := b.expr(e.Args[0])
	right, join := b.newBlockAt(), b.newBlockAt()
	b.assign(result, l)
	b.term(Term{Op: TermBranch, Args: []LocalID{l}, Targets: []BlockID{right, join}})
	b.cur = right
	b.assign(result, b.expr(e.Args[1]))
	b.jump(join)
	b.cur = join
	return result
}

// fallback lowers `a || b`: Bool, Option and Result are split inline,
// other types through their `branch` witness.
func (b *builder) fallback(e *hir.Expr) LocalID {
	n := e.Node
	l := b.expr(e.Args[0])
	lt := e.Args[0].Type
	if e.Ent != 0 {
		l = b.emit(Inst{Op: OpCall, Dst: b.temp(b.r.Types.Node(b.r.Fn(e.Ent).Sig).Elem), Ent: e.Ent, Args: []LocalID{l}, Node: n})
		lt = b.fn.Locals[l].Type
	}
	result := b.temp(e.Type)
	// The Option / Result temporary is owned by a scope that opens after
	// it is assigned and closes at the join, so both arms release it.
	if lt != sem.TyBool {
		b.pushScope()
		b.ownTemp(l)
	}
	hitBlock, missBlock, join := b.newBlockAt(), b.newBlockAt(), b.newBlockAt()
	var hit, miss LocalID
	var isHit LocalID
	ltn := b.r.Types.Node(lt)
	switch {
	case lt == sem.TyBool:
		isHit = l
	case ltn.Kind == sem.KNamed:
		hitName := "Some"
		if ltn.Ent == b.r.Types.ResultEnt() {
			hitName = "Ok"
		} else if ltn.Ent == b.r.LangItem("Branch") {
			hitName = "Hit"
		}
		isHit = b.emit(Inst{Op: OpIsVariant, Dst: b.temp(sem.TyBool), Ent: b.variantNamed(ltn.Ent, hitName), Args: []LocalID{l}, Node: n})
	}
	b.term(Term{Op: TermBranch, Args: []LocalID{isHit}, Targets: []BlockID{hitBlock, missBlock}})
	b.cur = hitBlock
	if lt == sem.TyBool {
		hit = l
	} else {
		hit = b.emit(Inst{Op: OpPayload, Dst: b.temp(ltn.Args[0]), Args: []LocalID{l}, Node: n})
	}
	b.assign(result, b.coerce(e.Coercion, n, hit))
	b.jump(join)
	b.cur = missBlock
	if ltn.Kind == sem.KNamed && len(ltn.Args) > 1 {
		miss = b.emit(Inst{Op: OpPayload, Dst: b.temp(ltn.Args[1]), Args: []LocalID{l}, Node: n})
	} else {
		miss = b.unit(n)
	}
	// The miss binding lives in the miss arm only: dropping it at the join
	// would read it on the hit path, which never assigned it.
	b.pushScope()
	if e.Param != nil {
		pl := b.declare(e.Param.Ent)
		b.assign(pl, miss)
	}
	b.assign(result, b.expr(e.Args[1]))
	b.exitScope()
	b.jump(join)
	b.cur = join
	if lt != sem.TyBool {
		b.exitScope()
	}
	return result
}

// pipe lowers `a |> (v) => body`: the lambda applied to a. Calls on the
// right of `|>` take the left operand as an ordinary first argument and
// never reach here.
func (b *builder) pipe(e *hir.Expr) LocalID {
	a := b.exprMoved(e.Args[0])
	fn := b.expr(e.Args[1])
	return b.emit(Inst{Op: OpCallValue, Dst: b.temp(e.Type), Args: []LocalID{fn, a}, Node: e.Node})
}

func (b *builder) try(e *hir.Expr) LocalID {
	n := e.Node
	v := b.expr(e.Args[0])
	rt := e.Args[0].Type
	if b.r.Types.Kind(rt) == sem.KRef {
		rt = b.r.Types.Node(rt).Elem
	}
	val, errT, _ := b.r.Types.IsResult(rt)
	ok := b.emit(Inst{Op: OpIsVariant, Dst: b.temp(sem.TyBool), Ent: b.variantNamed(b.r.Types.ResultEnt(), "Ok"), Args: []LocalID{v}, Node: n})
	yes, no := b.newBlockAt(), b.newBlockAt()
	b.term(Term{Op: TermBranch, Args: []LocalID{ok}, Targets: []BlockID{yes, no}})
	b.cur = no
	err := b.emit(Inst{Op: OpPayload, Dst: b.temp(errT), Args: []LocalID{v}, Node: n})
	if e.BoxType != 0 {
		err = b.emit(Inst{Op: OpBox, Dst: b.temp(b.r.Types.Iface(b.r.Types.ErrorEnt(), nil)), Type: e.BoxType, Args: []LocalID{err}, Node: n})
	}
	b.dropUnwrapped(v, n)
	b.failWith(n, err)
	b.cur = yes
	p := b.emit(Inst{Op: OpPayload, Dst: b.temp(val), Args: []LocalID{v}, Node: n})
	b.dropUnwrapped(v, n)
	return p
}

// dropUnwrapped releases the Result a `?` took apart. Extracting the payload
// gives it its own reference, and the Result temporary is read from both the
// Ok and the Err block, which is exactly the shape the back end refuses to
// release on its own.
func (b *builder) dropUnwrapped(v LocalID, n syntax.NodeID) {
	if b.fn.Locals[v].Ent == 0 {
		b.emit(Inst{Op: OpDrop, Args: []LocalID{v}, Node: n})
	}
}
