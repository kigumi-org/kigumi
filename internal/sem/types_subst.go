package sem

// Subst replaces KParam types by s; the result is interned.
func (tt *TypeTable) Subst(t TypeID, s map[EntityID]TypeID) TypeID {
	if len(s) == 0 || t == 0 {
		return t
	}
	n := tt.nodes[t]
	switch n.Kind {
	case KParam:
		if r, ok := s[n.Ent]; ok {
			return r
		}
		return t
	case KNamed, KIface, KFn:
		changed := false
		var args []TypeID
		for i, a := range n.Args {
			r := tt.Subst(a, s)
			if r != a && !changed {
				changed = true
				args = append([]TypeID(nil), n.Args[:i]...)
			}
			if changed {
				args = append(args, r)
			}
		}
		elem := tt.Subst(n.Elem, s)
		if !changed && elem == n.Elem {
			return t
		}
		if !changed {
			args = n.Args
		}
		return tt.Intern(typeNode{Kind: n.Kind, Flags: n.Flags, Ent: n.Ent, Elem: elem, Var: n.Var, Args: args})
	case KRef, KPtr:
		elem := tt.Subst(n.Elem, s)
		if elem == n.Elem {
			return t
		}
		return tt.Intern(typeNode{Kind: n.Kind, Flags: n.Flags, Elem: elem})
	}
	return t
}

// Contains reports whether needle occurs anywhere inside t.
func (tt *TypeTable) Contains(t, needle TypeID) bool {
	if t == needle {
		return true
	}
	if t == 0 {
		return false
	}
	n := tt.nodes[t]
	if n.Elem != 0 && tt.Contains(n.Elem, needle) {
		return true
	}
	for _, a := range n.Args {
		if tt.Contains(a, needle) {
			return true
		}
	}
	return false
}

// Params lists the type-parameter entities occurring in t, in first-seen order.
func (tt *TypeTable) Params(t TypeID) []EntityID {
	var out []EntityID
	seen := map[EntityID]bool{}
	var walk func(TypeID)
	walk = func(t TypeID) {
		if t == 0 {
			return
		}
		n := tt.nodes[t]
		if n.Kind == KParam {
			if !seen[n.Ent] {
				seen[n.Ent] = true
				out = append(out, n.Ent)
			}
			return
		}
		walk(n.Elem)
		for _, a := range n.Args {
			walk(a)
		}
	}
	walk(t)
	return out
}

// ContainsVar reports whether an unresolved inference variable occurs in t.
func (tt *TypeTable) ContainsVar(t TypeID) bool {
	if t == 0 {
		return false
	}
	n := tt.nodes[t]
	if n.Kind == KVar {
		return true
	}
	if tt.ContainsVar(n.Elem) {
		return true
	}
	for _, a := range n.Args {
		if tt.ContainsVar(a) {
			return true
		}
	}
	return false
}
