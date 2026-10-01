package sem

import "kigumi/internal/syntax"

// typesDefinitelyDiffer reports whether a and b can never be the same type
// at runtime. Types are hash-consed (TypeTable.Intern), so a == b
// already means equal; generics, interfaces, and refs stay "not sure" and
// keep the candidate, since over-approximating is safe but excluding one
// wrongly would not be.
func (r *Result) typesDefinitelyDiffer(a, b TypeID) bool {
	return a != b && r.typeFullyConcrete(a) && r.typeFullyConcrete(b)
}

// typeFullyConcrete reports whether t contains no type parameter,
// interface or unification variable anywhere in its structure, so its
// interned identity alone decides equality with another such type.
func (r *Result) typeFullyConcrete(t TypeID) bool {
	n := r.Types.Node(t)
	switch n.Kind {
	case KParam, KIface, KVar, KPoison:
		return false
	}
	if n.Elem != 0 && !r.typeFullyConcrete(n.Elem) {
		return false
	}
	for _, a := range n.Args {
		if !r.typeFullyConcrete(a) {
			return false
		}
	}
	return true
}

// literalArgDefinitelyMismatches is callArgsDefinitelyMismatch's
// counterpart for a non-identifier argument: a literal's value kind is just
// as statically known as a declared type, so it rules out the candidate the
// same way.
func (r *Result) literalArgDefinitelyMismatches(t *syntax.Tree, a syntax.NodeID, target TypeID) bool {
	if !r.typeFullyConcrete(target) {
		return false
	}
	for t.Kind(a) == syntax.Paren {
		a = syntax.NodeID(t.Nodes[a].Lhs)
	}
	switch {
	case isNumericLiteral(t, a):
		return !r.Types.IsNumeric(target)
	case t.Kind(a) == syntax.BoolLit:
		return target != TyBool
	case t.Kind(a) == syntax.CharLit:
		return target != TyChar
	case t.Kind(a) == syntax.StringLit:
		return target != TyString
	case t.Kind(a) == syntax.ByteStringLit:
		return target != TyBytes
	}
	return false
}
