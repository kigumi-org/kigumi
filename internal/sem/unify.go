package sem

import "kigumi/internal/syntax"

// varStore is the per-body table of inference variables: a
// union-find with bindings, pending literals and a trail for rollback.
type varStore struct {
	r       *Result
	binding []TypeID // 0 = unbound
	pending []uint8  // 0 none, 1 int, 2 float
	lits    [][]syntax.NodeID
	origin  []syntax.NodeID
	names   []string
	hint    []TypeID
	// lifetimeDefault is a lifetime var's settle() fallback,
	// 0 for an ordinary type-parameter variable (still cInferUnresolved).
	lifetimeDefault []TypeID
	first           int // first variable created since the last boundary
	trail           []uint32
}

func newVarStore(r *Result) *varStore { return &varStore{r: r} }

func (v *varStore) fresh(origin syntax.NodeID, name string) TypeID {
	v.binding = append(v.binding, 0)
	v.pending = append(v.pending, 0)
	v.lits = append(v.lits, nil)
	v.origin = append(v.origin, origin)
	v.names = append(v.names, name)
	v.hint = append(v.hint, 0)
	v.lifetimeDefault = append(v.lifetimeDefault, 0)
	return v.r.Types.Var(uint32(len(v.binding) - 1))
}

// fnHint is the fn shape a variable's type parameter demands through an Fn constraint.
func (v *varStore) fnHint(t TypeID) TypeID {
	if i, ok := v.index(v.resolve(t)); ok {
		return v.hint[i]
	}
	return 0
}

// poison binds every unbound variable inside t to TyPoison so that a
// reported error is not followed by an infer-unresolved error.
func (v *varStore) poison(t TypeID) {
	t = v.resolve(t)
	if t == 0 || !v.r.Types.ContainsVar(t) {
		return
	}
	n := v.r.Types.Node(t)
	if n.Kind == KVar {
		v.binding[n.Var] = TyPoison
		return
	}
	v.poison(n.Elem)
	for _, a := range n.Args {
		v.poison(a)
	}
}

func (v *varStore) boundary() { v.first = len(v.binding) }

func (v *varStore) index(t TypeID) (uint32, bool) {
	n := v.r.Types.Node(t)
	if n.Kind != KVar {
		return 0, false
	}
	return n.Var, true
}

// pendingIndex is index for an unbound variable that already carries a pending literal default.
func (v *varStore) pendingIndex(t TypeID) (uint32, bool) {
	i, ok := v.index(t)
	if !ok || v.pending[i] == 0 {
		return 0, false
	}
	return i, true
}

// resolve replaces bound variables inside t, recursively.
func (v *varStore) resolve(t TypeID) TypeID {
	if t == 0 || !v.r.Types.ContainsVar(t) {
		return t
	}
	tt := v.r.Types
	n := tt.Node(t)
	if n.Kind == KVar {
		if b := v.binding[n.Var]; b != 0 {
			return v.resolve(b)
		}
		return t
	}
	args := make([]TypeID, len(n.Args))
	for i, a := range n.Args {
		args[i] = v.resolve(a)
	}
	return tt.Intern(typeNode{Kind: n.Kind, Flags: n.Flags, Ent: n.Ent, Elem: v.resolve(n.Elem), Var: n.Var, Args: args})
}

func (v *varStore) bind(i uint32, t TypeID) { v.trail = append(v.trail, i); v.binding[i] = t }

// unify makes a and b equal, binding variables, and rolls back on failure.
// Untyped literals unify with a variable by making it a pending literal.
func (v *varStore) unify(a, b TypeID) bool {
	mark := len(v.trail)
	if v.unifyInner(a, b) {
		return true
	}
	for len(v.trail) > mark {
		i := v.trail[len(v.trail)-1]
		v.trail = v.trail[:len(v.trail)-1]
		v.binding[i] = 0
	}
	return false
}

func (v *varStore) unifyInner(a, b TypeID) bool {
	a, b = v.resolve(a), v.resolve(b)
	if a == b {
		return true
	}
	tt := v.r.Types
	na, nb := tt.Node(a), tt.Node(b)
	if na.Kind == KPoison || nb.Kind == KPoison {
		return true
	}
	if nb.Kind == KVar && na.Kind != KVar {
		a, b, na, nb = b, a, nb, na
	}
	if na.Kind == KVar {
		return v.bindVar(na.Var, b, nb)
	}
	if na.Kind == KUntyped && tt.IsNumeric(b) || nb.Kind == KUntyped && tt.IsNumeric(a) {
		return false
	}
	if na.Kind != nb.Kind || na.Flags != nb.Flags || na.Ent != nb.Ent || na.Var != nb.Var || len(na.Args) != len(nb.Args) {
		return false
	}
	if !v.unifyInner(na.Elem, nb.Elem) {
		return false
	}
	for i := range na.Args {
		if !v.unifyInner(na.Args[i], nb.Args[i]) {
			return false
		}
	}
	return true
}

func (v *varStore) bindVar(i uint32, t TypeID, n typeNode) bool {
	tt := v.r.Types
	switch {
	case n.Kind == KVar:
		if v.pending[i] != 0 && v.pending[n.Var] == 0 {
			v.pending[n.Var] = v.pending[i]
			v.lits[n.Var] = append(v.lits[n.Var], v.lits[i]...)
		} else if v.pending[i] == 2 && v.pending[n.Var] == 1 {
			v.pending[n.Var] = 2
		}
		v.bind(i, t)
		return true
	case n.Kind == KUntyped:
		kind := uint8(1)
		if t == TyUntypedFloat {
			kind = 2
		}
		if v.pending[i] < kind {
			v.pending[i] = kind
		}
		return true
	case v.pending[i] != 0:
		if !tt.IsNumeric(t) || (v.pending[i] == 2 && tt.IsInteger(t)) {
			return false
		}
		v.bind(i, t)
		return true
	}
	if tt.Contains(t, tt.Var(i)) {
		return false
	}
	v.bind(i, t)
	return true
}

// unifiable reports whether unify would succeed, without binding.
func (v *varStore) unifiable(a, b TypeID) bool {
	mark := len(v.trail)
	ok := v.unifyInner(a, b)
	for len(v.trail) > mark {
		i := v.trail[len(v.trail)-1]
		v.trail = v.trail[:len(v.trail)-1]
		v.binding[i] = 0
	}
	return ok
}
