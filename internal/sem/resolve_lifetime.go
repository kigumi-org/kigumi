package sem

import (
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// `&'a T` and `&T` share one interned type; the tag lives on the
// declaration instead.
func (r *Result) resolveRefType(f FileID, scope ScopeID, n syntax.NodeID, node syntax.Node, pos typePos) TypeID {
	t := r.tree(f)
	var lt EntityID
	if t.Toks[node.Tok].Kind == token.Lifetime {
		lt = r.resolveLifetimeName(f, scope, n, t.TokText(node.Tok))
	}
	switch {
	case pos == posParam || pos == posLocal:
	case pos == posField && lt == 0:
		r.errAt(f, n, cLifetimeFieldAnnot)
	case pos == posReturn && lt == 0:
		r.errAt(f, n, cLifetimeReturnAnnot)
	case pos != posField && pos != posReturn:
		r.errAt(f, n, cBorrowPosition, typePosNames[pos])
	}
	elem := r.resolveType(f, scope, syntax.NodeID(node.Rhs), posTypeArg)
	return r.Types.Ref(elem, node.Lhs&syntax.FlagMut != 0)
}

// Only declareGenerics binds such a name, so a hit is always a lifetime
// parameter.
func (r *Result) resolveLifetimeName(f FileID, scope ScopeID, n syntax.NodeID, name string) EntityID {
	b, _, ok := r.lookup(scope, name)
	if !ok || b.Ent == 0 {
		r.errAt(f, n, cLifetimeUnresolved, name)
		return 0
	}
	return b.Ent
}

// resolveType already validated and reported this; a failed lookup here
// yields 0 (as if untagged) rather than a duplicate diagnostic. Only the
// record's first lifetime argument is a valid elision source.
func (r *Result) explicitLifetimeOf(f FileID, scope ScopeID, n syntax.NodeID) EntityID {
	t := r.tree(f)
	switch t.Kind(n) {
	case syntax.TypeRef:
		node := t.Nodes[n]
		if t.Toks[node.Tok].Kind != token.Lifetime {
			return 0
		}
		return r.lookupLifetimeName(scope, t.TokText(node.Tok))
	case syntax.TypePath:
		node := t.Nodes[n]
		if node.Rhs == 0 {
			return 0
		}
		for _, a := range t.Children(syntax.NodeID(node.Rhs)) {
			if t.Kind(a) == syntax.TypeLifetime {
				return r.lookupLifetimeName(scope, t.TokText(t.Nodes[a].Tok))
			}
		}
	}
	return 0
}

// Unlike an ordinary type-param var, never reports cInferUnresolved; settles
// to fallback's KParam instead, since a return-only lifetime may never be
// touched by unification.
func (v *varStore) freshLifetime(origin syntax.NodeID, param, fallback EntityID) TypeID {
	t := v.fresh(origin, v.r.Entities[param].Name)
	i, _ := v.index(t)
	v.lifetimeDefault[i] = v.r.Types.Param(fallback)
	return t
}

// Returns 0 for none or more than one lifetime parameter.
func (r *Result) soleLifetimeParam(fn EntityID) EntityID {
	if fn == 0 || r.Entities[fn].Kind != EntFn {
		return 0
	}
	var sole EntityID
	for _, p := range r.Fn(fn).TypeParams {
		if !r.typeParam(p).IsLifetime {
			continue
		}
		if sole != 0 {
			return 0
		}
		sole = p
	}
	return sole
}

func (r *Result) lookupLifetimeName(scope ScopeID, name string) EntityID {
	b, _, ok := r.lookup(scope, name)
	if !ok {
		return 0
	}
	return b.Ent
}

// Unlike a bare reference's tag, this value becomes part of the resulting
// TypeID: two instantiations of the same generic record with different
// lifetimes are meant to stay distinct types.
func (r *Result) resolveLifetimeArg(f FileID, scope ScopeID, n syntax.NodeID) TypeID {
	t := r.tree(f)
	if t.Kind(n) != syntax.TypeLifetime {
		r.errAt(f, n, cLifetimeArgKind, "this", "a lifetime", "a type")
		return TyPoison
	}
	node := t.Nodes[n]
	ent := r.resolveLifetimeName(f, scope, n, t.TokText(node.Tok))
	if ent == 0 {
		return TyPoison
	}
	return r.Types.Param(ent)
}
