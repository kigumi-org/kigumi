package printer

import (
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func (p *printer) exprList(list syntax.NodeID) {
	for i, e := range p.t.Children(list) {
		if i > 0 {
			p.write(", ")
		}
		p.withIndent(func() { p.expr(e) })
	}
}

func (p *printer) expr(id syntax.NodeID) {
	if id == 0 {
		return
	}
	n := p.node(id)
	switch n.Kind {
	case syntax.Ident, syntax.IntLit, syntax.FloatLit, syntax.CharLit, syntax.BoolLit:
		p.write(p.tok(n.Tok))
	case syntax.StringLit, syntax.ByteStringLit:
		p.write(p.tok(n.Tok))
	case syntax.ShellLit:
		p.write("$" + p.tok(n.Lhs))
	case syntax.Paren:
		inner := syntax.NodeID(n.Lhs)
		if p.t.Kind(inner) == syntax.Paren {
			p.expr(inner)
			return
		}
		p.write("(")
		p.expr(inner)
		p.write(")")
	case syntax.Unary:
		p.write(p.tok(n.Tok))
		p.expr(syntax.NodeID(n.Lhs))
	case syntax.Binary:
		p.binary(id)
	case syntax.IsExpr:
		p.expr(syntax.NodeID(n.Lhs))
		p.write(" is ")
		p.pattern(syntax.NodeID(n.Rhs))
	case syntax.CallExpr:
		p.expr(syntax.NodeID(n.Lhs))
		p.write("(")
		p.exprList(syntax.NodeID(n.Rhs))
		p.write(")")
	case syntax.WsCallExpr:
		p.wsCall(id)
	case syntax.BracketExpr:
		p.expr(syntax.NodeID(n.Lhs))
		p.write("[")
		p.exprList(syntax.NodeID(n.Rhs))
		p.write("]")
	case syntax.TypeFn, syntax.TypePtr:
		p.typ(id)
	case syntax.MemberExpr:
		p.expr(syntax.NodeID(n.Lhs))
		p.member(n.Tok, ".")
	case syntax.OptMemberExpr:
		p.expr(syntax.NodeID(n.Lhs))
		p.member(n.Tok, "?.")
	case syntax.Spread:
		p.write("...")
		p.expr(syntax.NodeID(n.Lhs))
	case syntax.NamedArg:
		p.write(p.tok(n.Tok) + ": ")
		p.expr(syntax.NodeID(n.Lhs))
	case syntax.TryExpr:
		p.expr(syntax.NodeID(n.Lhs))
		p.write("?")
	case syntax.RecordLit:
		p.recordLit(id)
	case syntax.TupleLit:
		p.tupleLit(id)
	case syntax.UnitLit:
		p.write("()")
	case syntax.Lambda:
		p.lambda(id)
	case syntax.Block:
		p.block(id)
	case syntax.IfExpr:
		p.ifExpr(id)
	case syntax.IfLetExpr:
		p.ifLet(id)
	case syntax.MatchExpr:
		p.matchExpr(id)
	case syntax.ForExpr:
		p.forExpr(id)
	case syntax.ReturnExpr, syntax.FailExpr, syntax.BreakExpr, syntax.ContinueExpr,
		syntax.AwaitExpr, syntax.ComptimeExpr, syntax.UnsafeExpr, syntax.MoveExpr:
		p.write(p.tok(n.Tok))
		if n.Lhs != 0 {
			p.write(" ")
			p.expr(syntax.NodeID(n.Lhs))
		}
	case syntax.AsmExpr:
		p.asmBlock(id)
	case syntax.AllocatorExpr:
		p.write("allocator ")
		p.expr(syntax.NodeID(n.Lhs))
		p.write(" ")
		p.block(syntax.NodeID(n.Rhs))
	case syntax.ContractExpr:
		p.write(modsText(n.Lhs) + " ")
		p.block(syntax.NodeID(n.Rhs))
	case syntax.BorrowExpr:
		p.write("&")
		if n.Lhs&syntax.FlagMut != 0 {
			p.write("mut ")
		}
		p.expr(syntax.NodeID(n.Rhs))
	}
}

// member prints a `.name` segment. A segment that started a new line in the
// source keeps its own line, indented once for the whole chain.
func (p *printer) member(nameTok uint32, dot string) {
	if nameTok >= 2 && p.t.Toks[nameTok-2].Kind == token.Newline {
		p.newline()
		if !p.chain {
			p.indent++
			p.chain = true
		}
	}
	p.write(dot + p.tok(nameTok))
}

// wsCall keeps `f x` only for a bare identifier callee; member callees
// become ordinary calls.
func (p *printer) wsCall(id syntax.NodeID) {
	n := p.node(id)
	callee := syntax.NodeID(n.Lhs)
	if p.t.Kind(callee) == syntax.Ident {
		p.expr(callee)
		p.write(" ")
		p.exprList(syntax.NodeID(n.Rhs))
		return
	}
	p.expr(callee)
	p.write("(")
	p.exprList(syntax.NodeID(n.Rhs))
	p.write(")")
}
