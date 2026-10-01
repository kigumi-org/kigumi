package interp

import (
	"math/big"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// expr evaluates an expression and applies its recorded coercion.
func (fr *frame) expr(n syntax.NodeID) (Value, *ctrl) {
	v, c := fr.exprRaw(n)
	if c != nil {
		return nil, c
	}
	// A fallback lifts only its hit value (eval_pipe.go); its miss arm is
	// already checked at the lifted type, so the node's coercion must not
	// run again here.
	if fr.t.Kind(n) == syntax.Binary && fr.info.Calls[n].Kind == sem.CallFallback {
		return v, nil
	}
	// fr.block already applied the same node's coercion to a tailless
	// block; applying it again here would double-wrap it.
	if fr.t.Kind(n) == syntax.Block {
		return v, nil
	}
	return fr.coerce(n, v), nil
}

func (fr *frame) exprRaw(n syntax.NodeID) (Value, *ctrl) {
	node := fr.t.Nodes[n]
	switch node.Kind {
	case syntax.Ident:
		return fr.ident(n)
	case syntax.IntLit, syntax.FloatLit:
		return fr.literal(n), nil
	case syntax.CharLit:
		r, _ := syntax.DecodeChar(fr.t.TokText(node.Tok))
		return Char(r), nil
	case syntax.BoolLit:
		return Bool(fr.t.TokText(node.Tok) == "true"), nil
	case syntax.StringLit:
		return fr.stringLit(n)
	case syntax.ByteStringLit:
		return Bytes(syntax.DecodeByteString(fr.t.TokText(node.Tok))), nil
	case syntax.ShellLit:
		return fr.shellLit(n)
	case syntax.Paren:
		return fr.expr(syntax.NodeID(node.Lhs))
	case syntax.Unary:
		return fr.unary(n)
	case syntax.Binary:
		return fr.binary(n)
	case syntax.IsExpr:
		return fr.isExpr(n)
	case syntax.CallExpr, syntax.WsCallExpr:
		return fr.call(n)
	case syntax.BracketExpr:
		return fr.index(n)
	case syntax.MemberExpr:
		return fr.member(n)
	case syntax.OptMemberExpr:
		return fr.optMember(n)
	case syntax.TryExpr:
		return fr.try(n)
	case syntax.RecordLit:
		return fr.recordLit(n)
	case syntax.TupleLit:
		return fr.tupleLit(n)
	case syntax.UnitLit:
		return Unit{}, nil
	case syntax.Lambda:
		return fr.lambda(n), nil
	case syntax.Block:
		return fr.block(n)
	case syntax.IfExpr:
		return fr.ifExpr(n)
	case syntax.IfLetExpr:
		return fr.ifLet(n)
	case syntax.MatchExpr:
		return fr.matchExpr(n)
	case syntax.ForExpr:
		return fr.forExpr(n)
	case syntax.ReturnExpr:
		if node.Lhs == 0 {
			return nil, &ctrl{kind: ctrlReturn, val: Unit{}}
		}
		v, c := fr.expr(syntax.NodeID(node.Lhs))
		if c != nil {
			return nil, c
		}
		return nil, &ctrl{kind: ctrlReturn, val: fr.transfer(syntax.NodeID(node.Lhs), v)}
	case syntax.FailExpr:
		v, c := fr.expr(syntax.NodeID(node.Lhs))
		if c != nil {
			return nil, c
		}
		return nil, &ctrl{kind: ctrlFail, val: v}
	case syntax.BreakExpr:
		if node.Lhs == 0 {
			return nil, &ctrl{kind: ctrlBreak, val: Unit{}}
		}
		v, c := fr.expr(syntax.NodeID(node.Lhs))
		if c != nil {
			return nil, c
		}
		return nil, &ctrl{kind: ctrlBreak, val: fr.transfer(syntax.NodeID(node.Lhs), v)}
	case syntax.ContinueExpr:
		return nil, &ctrl{kind: ctrlContinue}
	case syntax.UnsafeExpr, syntax.ComptimeExpr:
		return fr.expr(syntax.NodeID(node.Lhs))
	case syntax.AsmExpr:
		fr.panicAt(n, "inline assembly needs `kigumi build`")
		return nil, nil
	case syntax.AwaitExpr:
		v, c := fr.expr(syntax.NodeID(node.Lhs))
		if c != nil {
			return nil, c
		}
		return fr.await(n, v)
	case syntax.ContractExpr:
		return fr.block(syntax.NodeID(node.Rhs))
	case syntax.AllocatorExpr:
		if _, c := fr.expr(syntax.NodeID(node.Lhs)); c != nil {
			return nil, c
		}
		return fr.block(syntax.NodeID(node.Rhs))
	case syntax.BorrowExpr:
		return fr.borrow(syntax.NodeID(node.Rhs))
	case syntax.MoveExpr:
		return fr.expr(syntax.NodeID(node.Lhs))
	case syntax.Spread:
		return fr.expr(syntax.NodeID(node.Lhs))
	}
	fr.panicAt(n, "cannot evaluate %s", node.Kind)
	return nil, nil
}

func (fr *frame) ident(n syntax.NodeID) (Value, *ctrl) {
	ent := fr.info.Uses[n]
	in := fr.in
	e := in.r.Entity(ent)
	switch e.Kind {
	case sem.EntLocal, sem.EntParam:
		v := fr.cell(ent).V
		if _, moved := v.(Moved); moved {
			fr.panicAt(n, "use of moved value `%s`", e.Name)
		}
		return v, nil
	case sem.EntConst:
		return in.constValue(ent, fr.typeOf(n)), nil
	case sem.EntFn:
		return &FnItem{Fn: ent}, nil
	case sem.EntVariant:
		return &Variant{Type: fr.typeOf(n), V: ent}, nil
	case sem.EntIntrinsic:
		return hostValue(), nil
	}
	fr.panicAt(n, "`%s` is not a value", e.Name)
	return nil, nil
}

// hostValue is the runtime value of the entry-only `host` capability,
// whether reached as a bare identifier or bound to an explicit
// `fn main(host: Host)` parameter.
func hostValue() Value { return &Opaque{Kind: "Host"} }

func (in *Interp) constValue(ent sem.EntityID, t sem.TypeID) Value {
	lit, _ := in.r.ConstValueOf(ent)
	switch lit.Kind {
	case sem.LitInt:
		if in.r.Types.IsFloat(t) {
			f, _ := new(big.Float).SetInt(lit.Int).Float64()
			return mkFloat(in, f, t)
		}
		if t == 0 || in.r.Types.Kind(t) == sem.KUntyped {
			t = sem.TyI64
		}
		return Int{V: lit.Int.Int64(), T: t}
	case sem.LitFloat:
		f, _ := lit.Float.Float64()
		if t == 0 || in.r.Types.Kind(t) == sem.KUntyped {
			t = sem.TyF64
		}
		return mkFloat(in, f, t)
	case sem.LitString:
		return Str(lit.Str)
	case sem.LitBytes:
		return Bytes(lit.Str)
	case sem.LitBool:
		return Bool(lit.Bool)
	case sem.LitChar:
		return Char(lit.Char)
	}
	return Unit{}
}
