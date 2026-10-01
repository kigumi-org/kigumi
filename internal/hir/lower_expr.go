package hir

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// A block already carries its own coercion on Block.Coercion (set by
// l.block for the same node), so the wrapping BlockExpr must not record
// it a second time.
func (l *lowerer) expr(n syntax.NodeID) *Expr {
	e := l.exprRaw(n)
	if e.Kind != BlockExpr {
		e.Coercion = l.coercion(n)
	}
	return e
}

func (l *lowerer) node(n syntax.NodeID, k ExprKind) *Expr {
	return &Expr{Kind: k, Type: l.typeOf(n), Loc: l.loc(n), Node: n}
}

func (l *lowerer) wrap(n, inner syntax.NodeID) *Expr {
	out := l.node(n, Wrap)
	out.Args = []*Expr{l.expr(inner)}
	return out
}

func (l *lowerer) exprRaw(n syntax.NodeID) *Expr {
	node := l.t.Nodes[n]
	lhs := syntax.NodeID(node.Lhs)
	switch node.Kind {
	case syntax.Ident:
		return l.ident(n)
	case syntax.IntLit, syntax.FloatLit:
		return l.literal(n)
	case syntax.CharLit:
		r, _ := syntax.DecodeChar(l.t.TokText(node.Tok))
		out := l.node(n, Lit)
		out.Type, out.Lit = sem.TyChar, sem.Literal{Kind: sem.LitChar, Char: r}
		return out
	case syntax.BoolLit:
		out := l.node(n, Lit)
		out.Type, out.Lit = sem.TyBool, sem.Literal{Kind: sem.LitBool, Bool: l.t.TokText(node.Tok) == "true"}
		return out
	case syntax.StringLit:
		return l.stringLit(n)
	case syntax.ByteStringLit:
		return l.byteStringLit(n)
	case syntax.ShellLit:
		return l.shellLit(n)
	case syntax.Paren, syntax.UnsafeExpr, syntax.Spread:
		return l.wrap(n, lhs)
	case syntax.Unary:
		if lit, ok := l.r.LiteralOf(l.t, n); ok && lit.Kind != sem.LitString {
			return l.literal(n)
		}
		out := l.node(n, Unary)
		out.Name, out.Args = l.t.TokText(node.Tok), []*Expr{l.expr(lhs)}
		if call, ok := l.info.Calls[n]; ok && call.Kind == sem.CallOperator {
			out.Ent = call.Callee
		}
		return out
	case syntax.Binary:
		return l.binary(n)
	case syntax.IsExpr:
		out := l.node(n, Is)
		out.Args, out.Pat = []*Expr{l.expr(lhs)}, l.pattern(syntax.NodeID(node.Rhs))
		return out
	case syntax.CallExpr, syntax.WsCallExpr:
		return l.call(n)
	case syntax.BracketExpr:
		out := l.node(n, Index)
		out.Args = []*Expr{l.expr(lhs), l.expr(l.t.Children(syntax.NodeID(node.Rhs))[0])}
		return out
	case syntax.MemberExpr:
		return l.member(n)
	case syntax.OptMemberExpr:
		out := l.node(n, OptField)
		out.Args, out.Index = []*Expr{l.expr(lhs)}, l.r.Field(l.info.Uses[n]).Index
		return out
	case syntax.TryExpr:
		out := l.node(n, Try)
		out.Args = []*Expr{l.expr(lhs)}
		if call, ok := l.info.Calls[n]; ok && len(call.Inst) == 1 {
			out.BoxType = call.Inst[0]
		}
		return out
	case syntax.RecordLit:
		return l.record(n)
	case syntax.TupleLit:
		return l.tupleLit(n)
	case syntax.UnitLit:
		return l.node(n, Unit)
	case syntax.Lambda:
		out := l.node(n, Lambda)
		out.Ent = l.info.Defs[n]
		out.Fn = l.closure(out.Ent, syntax.NodeID(node.Rhs))
		return out
	case syntax.Block:
		out := l.node(n, BlockExpr)
		out.Block = l.block(n)
		return out
	case syntax.IfExpr:
		out := l.node(n, If)
		out.Cond, out.Then = l.expr(syntax.NodeID(l.t.Slot(n, "cond"))), l.block(syntax.NodeID(l.t.Slot(n, "then")))
		if els := syntax.NodeID(l.t.Slot(n, "else")); els != 0 {
			out.Else = l.expr(els)
		}
		return out
	case syntax.IfLetExpr, syntax.MatchExpr, syntax.ForExpr:
		return l.control(n)
	case syntax.ReturnExpr:
		out := l.node(n, Return)
		if lhs != 0 {
			out.Args = []*Expr{l.expr(lhs)}
		}
		return out
	case syntax.FailExpr:
		out := l.node(n, Fail)
		out.Args = []*Expr{l.expr(lhs)}
		return out
	case syntax.BreakExpr:
		out := l.node(n, Break)
		if lhs != 0 {
			out.Args = []*Expr{l.expr(lhs)}
		}
		return out
	case syntax.ContinueExpr:
		return l.node(n, Continue)
	case syntax.AsmExpr:
		out := l.node(n, Asm)
		if info := l.info.Asm[n]; info != nil {
			for _, op := range info.Operands {
				if (op.Kind == sem.AsmIn || op.Kind == sem.AsmInOut) && op.Expr != 0 {
					out.Args = append(out.Args, l.expr(op.Expr))
				}
			}
		}
		return out
	case syntax.AwaitExpr:
		out := l.node(n, Await)
		out.Args = []*Expr{l.expr(lhs)}
		return out
	case syntax.ComptimeExpr:
		if lit, ok := l.r.LiteralOf(l.t, n); ok {
			out := l.node(n, Lit)
			out.Lit = lit
			return out
		}
		out := l.node(n, Comptime)
		out.Args = []*Expr{l.expr(lhs)}
		return out
	case syntax.ContractExpr:
		out := l.node(n, BlockExpr)
		out.Block = l.block(syntax.NodeID(node.Rhs))
		return out
	case syntax.AllocatorExpr:
		out := l.node(n, Allocator)
		out.Args, out.Block = []*Expr{l.expr(lhs)}, l.block(syntax.NodeID(node.Rhs))
		return out
	case syntax.BorrowExpr:
		out := l.node(n, Borrow)
		out.Args, out.Mut = []*Expr{l.expr(syntax.NodeID(node.Rhs))}, node.Lhs&syntax.FlagMut != 0
		return out
	case syntax.MoveExpr:
		return l.move(n, lhs)
	}
	out := l.node(n, Panic)
	out.Name = "unsupported expression " + node.Kind.String()
	return out
}
