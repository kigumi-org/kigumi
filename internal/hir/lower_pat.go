package hir

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func (l *lowerer) pattern(p syntax.NodeID) *Pat {
	node := l.t.Nodes[p]
	out := &Pat{Type: l.typeOf(p), Node: p, Borrow: l.info.Pats[p].Borrow, Mut: l.info.Pats[p].Mut}
	switch node.Kind {
	case syntax.PatWildcard:
		out.Kind = PatWild
	case syntax.PatBind:
		out.Kind, out.Ent = PatBind, l.info.Defs[p]
	case syntax.PatLit:
		out.Kind, out.Lit = PatLit, l.expr(syntax.NodeID(node.Lhs))
	case syntax.PatCtor:
		out.Kind, out.Ent = PatCtor, l.info.Uses[p]
		out.IsType = l.r.Entity(out.Ent).Kind != sem.EntVariant
		for _, s := range l.t.Children(syntax.NodeID(node.Rhs)) {
			out.Subs = append(out.Subs, l.pattern(s))
		}
	case syntax.PatRecord:
		out.Kind, out.Ent = PatRecord, l.info.Uses[p]
		out.Variant = l.r.Entity(out.Ent).Kind == sem.EntVariant
		for _, f := range l.t.Children(syntax.NodeID(node.Rhs)) {
			out.Fields = append(out.Fields, l.patField(f, out.Ent))
		}
	case syntax.PatTuple:
		out.Kind, out.Ent = PatRecord, l.info.Uses[p]
		for i, s := range l.t.Children(p) {
			out.Fields = append(out.Fields, &PatField{Node: s, Type: l.typeOf(s), Index: i, Sub: l.pattern(s)})
		}
	case syntax.PatOr:
		out.Kind = PatOr
		for _, alt := range l.t.Children(p) {
			out.Subs = append(out.Subs, l.pattern(alt))
		}
	case syntax.PatRange:
		out.Kind = PatRange
		out.Lo, out.Hi = l.expr(syntax.NodeID(node.Lhs)), l.expr(syntax.NodeID(node.Rhs))
		out.Inclusive = l.t.Toks[node.Tok].Kind == token.DotDotEq
	default:
		out.Kind = PatNever
	}
	return out
}

// patField resolves the field a record-pattern entry names: a payload
// position of a variant, or a field index of a record.
func (l *lowerer) patField(f syntax.NodeID, owner sem.EntityID) *PatField {
	fn := l.t.Nodes[f]
	name := l.t.TokText(fn.Tok)
	out := &PatField{Node: f, Type: l.typeOf(f)}
	if fn.Lhs != 0 {
		out.Sub = l.pattern(syntax.NodeID(fn.Lhs))
	} else {
		out.Bind = l.info.Defs[f]
	}
	if l.r.Entity(owner).Kind == sem.EntVariant {
		for i, n := range l.r.Variant(owner).Names {
			if n == name {
				out.Payload, out.Index = true, i
				return out
			}
		}
	}
	out.Index = l.r.Field(l.r.FindField(owner, name)).Index
	return out
}
