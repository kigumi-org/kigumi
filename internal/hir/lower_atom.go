package hir

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// move lowers `move place`; a Copy field stays Field, not
// FieldMove, since the ownership analysis never tracks that slot, and
// clearing it would lose a value still readable through it.
func (l *lowerer) move(n, target syntax.NodeID) *Expr {
	for l.t.Kind(target) == syntax.Paren {
		target = syntax.NodeID(l.t.Nodes[target].Lhs)
	}
	if l.t.Kind(target) != syntax.MemberExpr {
		return l.expr(target)
	}
	fld := l.r.Field(l.info.Uses[target])
	base := l.expr(syntax.NodeID(l.t.Nodes[target].Lhs))
	kind := FieldMove
	if l.r.IsCopy(fld.Type) {
		kind = Field
	}
	out := l.node(target, kind)
	out.Args, out.Index = []*Expr{base}, fld.Index
	return out
}

func (l *lowerer) literal(n syntax.NodeID) *Expr {
	lit, _ := l.r.LiteralOf(l.t, n)
	out := l.node(n, Lit)
	out.Lit = lit
	if out.Type == 0 || l.r.Types.Kind(out.Type) == sem.KUntyped {
		out.Type = sem.TyI64
		if lit.Kind == sem.LitFloat {
			out.Type = sem.TyF64
		}
	}
	return out
}

func (l *lowerer) ident(n syntax.NodeID) *Expr {
	ent := l.info.Uses[n]
	e := l.r.Entity(ent)
	switch e.Kind {
	case sem.EntLocal, sem.EntParam:
		out := l.node(n, Local)
		out.Ent = ent
		return out
	case sem.EntConst:
		out := l.node(n, Lit)
		out.Lit, _ = l.r.ConstValueOf(ent)
		if out.Type == 0 || l.r.Types.Kind(out.Type) == sem.KUntyped {
			out.Type = e.Type
		}
		return out
	case sem.EntFn:
		out := l.node(n, FnItem)
		out.Ent = ent
		return out
	case sem.EntVariant:
		out := l.node(n, Variant)
		out.Ent = ent
		return out
	case sem.EntIntrinsic:
		return l.node(n, Host)
	}
	out := l.node(n, Panic)
	out.Name = "not a value"
	return out
}

func (l *lowerer) member(n syntax.NodeID) *Expr {
	node := l.t.Nodes[n]
	ent := l.info.Uses[n]
	if call, ok := l.info.Calls[n]; ok && call.Kind == sem.CallMethodValue {
		out := l.node(n, MethodValue)
		out.Ent, out.Args = call.Callee, []*Expr{l.expr(syntax.NodeID(node.Lhs))}
		return out
	}
	switch l.r.Entity(ent).Kind {
	case sem.EntField:
		out := l.node(n, Field)
		out.Args, out.Index = []*Expr{l.expr(syntax.NodeID(node.Lhs))}, l.r.Field(ent).Index
		return out
	case sem.EntFn:
		out := l.node(n, FnItem)
		out.Ent = ent
		return out
	case sem.EntConst:
		out := l.node(n, Lit)
		out.Lit, _ = l.r.ConstValueOf(ent)
		return out
	case sem.EntVariant:
		out := l.node(n, Variant)
		out.Ent = ent
		return out
	}
	out := l.node(n, Panic)
	out.Name = "not a value"
	return out
}

func (l *lowerer) stringLit(n syntax.NodeID) *Expr {
	src := l.t.File.Src
	out := l.node(n, Interp)
	text := ""
	for _, p := range l.t.StringParts(n) {
		if p.Expr == 0 {
			piece := syntax.DecodeString(string(src[p.Text.Start:p.Text.End]))
			out.Strs = append(out.Strs, piece)
			text += piece
			continue
		}
		out.Strs = append(out.Strs, "")
		out.Args = append(out.Args, l.expr(p.Expr))
	}
	if len(out.Args) == 0 {
		out.Kind, out.Type, out.Lit, out.Strs = Lit, sem.TyString, sem.Literal{Kind: sem.LitString, Str: text}, nil
	}
	return out
}

func (l *lowerer) byteStringLit(n syntax.NodeID) *Expr {
	out := l.node(n, Lit)
	out.Type, out.Lit = sem.TyBytes, sem.Literal{Kind: sem.LitBytes, Str: string(syntax.DecodeByteString(l.t.TokText(l.t.Nodes[n].Tok)))}
	return out
}
