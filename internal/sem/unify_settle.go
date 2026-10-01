package sem

import "kigumi/internal/syntax"

// settle runs at a statement boundary: unbound variables become their
// literal default or an infer-unresolved error, and pending literal nodes
// are range-checked against the final type.
func (v *varStore) settle(c *checker) {
	for i := v.first; i < len(v.binding); i++ {
		if v.binding[i] != 0 {
			continue
		}
		switch {
		case v.pending[i] == 1:
			v.binding[i] = TyI64
		case v.pending[i] == 2:
			v.binding[i] = TyF64
		case v.lifetimeDefault[i] != 0:
			v.binding[i] = v.lifetimeDefault[i]
		default:
			c.errAt(v.origin[i], cInferUnresolved, v.names[i], "explicit type arguments or an annotation")
			v.binding[i] = TyPoison
		}
	}
	for i := v.first; i < len(v.binding); i++ {
		final := v.resolve(v.binding[i])
		for _, lit := range v.lits[i] {
			if c.checkLiteralFits(lit, final) {
				c.setType(lit, final)
			}
		}
	}
}

// settleLiterals binds the pending literal variables inside t to their
// defaults now, for positions that need a concrete type early.
func (v *varStore) settleLiterals(t TypeID) TypeID {
	t = v.resolve(t)
	if t == 0 || !v.r.Types.ContainsVar(t) {
		return t
	}
	n := v.r.Types.Node(t)
	if n.Kind == KVar {
		switch v.pending[n.Var] {
		case 1:
			v.bind(n.Var, TyI64)
		case 2:
			v.bind(n.Var, TyF64)
		}
		return v.resolve(t)
	}
	for _, a := range n.Args {
		v.settleLiterals(a)
	}
	v.settleLiterals(n.Elem)
	return v.resolve(t)
}

// defaulted resolves t for display, replacing pending literal variables
// with the type they would settle to.
func (v *varStore) defaulted(t TypeID) TypeID {
	t = v.resolve(t)
	if t == 0 || !v.r.Types.ContainsVar(t) {
		return t
	}
	tt := v.r.Types
	n := tt.Node(t)
	if n.Kind == KVar {
		switch v.pending[n.Var] {
		case 1:
			return TyI64
		case 2:
			return TyF64
		}
		return t
	}
	args := make([]TypeID, len(n.Args))
	for i, a := range n.Args {
		args[i] = v.defaulted(a)
	}
	return tt.Intern(typeNode{Kind: n.Kind, Flags: n.Flags, Ent: n.Ent, Elem: v.defaulted(n.Elem), Var: n.Var, Args: args})
}

func (v *varStore) notePending(i uint32, lit syntax.NodeID) {
	v.lits[i] = append(v.lits[i], lit)
}

// literalLabel resolves t for a mismatch diagnostic (E400): unlike defaulted, a bare
// unsettled pending variable keeps its untyped literal type so the two sides don't render under the same name.
func (v *varStore) literalLabel(t TypeID) TypeID {
	if i, ok := v.index(v.resolve(t)); ok {
		switch v.pending[i] {
		case 1:
			return TyUntypedInt
		case 2:
			return TyUntypedFloat
		}
	}
	return v.defaulted(t)
}
