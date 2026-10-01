package hir

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// protoBinary lowers a comparison the checker routed to a protocol method:
// `a == b` is `a.equals(&b)`, `a < b` is `a.compareTo(&b) is Less`, and
// the negated forms wrap them in `!`.
func (l *lowerer) protoBinary(n syntax.NodeID, op string, call sem.CallInfo, lhs, rhs syntax.NodeID) *Expr {
	left, right := l.expr(lhs), l.expr(rhs)
	if l.r.Types.Kind(right.Type) != sem.KRef {
		elem := right.Type
		if l.r.Types.Kind(elem) == sem.KUntyped {
			elem = left.Type
		}
		b := l.node(rhs, Borrow)
		b.Type, b.Args = l.r.Types.Ref(elem, false), []*Expr{right}
		right = b
	}
	mc := l.node(n, MethodCall)
	mc.Ent, mc.Args, mc.Witness, mc.Passes = call.Callee, []*Expr{left, right}, call.Witness, call.Passes
	var out *Expr
	switch op {
	case "==", "!=":
		out = mc
	default:
		ordering := l.r.PackageMember(l.r.PackageByPath("std/prelude"), "Ordering")
		mc.Type = l.r.Types.Named(ordering, nil)
		variant := "Less"
		if op == ">" || op == "<=" {
			variant = "Greater"
		}
		is := l.node(n, Is)
		is.Args = []*Expr{mc}
		is.Pat = &Pat{Kind: PatCtor, Ent: l.variantNamed(ordering, variant), Type: mc.Type, Node: n}
		out = is
	}
	if op == "!=" || op == "<=" || op == ">=" {
		neg := l.node(n, Unary)
		neg.Name, neg.Args = "!", []*Expr{out}
		return neg
	}
	return out
}

func (l *lowerer) variantNamed(adt sem.EntityID, name string) sem.EntityID {
	for _, v := range l.r.TypeDecl(adt).Variants {
		if l.r.Entity(v).Name == name {
			return v
		}
	}
	return 0
}
