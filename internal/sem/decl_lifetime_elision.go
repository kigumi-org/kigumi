package sem

import (
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// resolveReturnType resolves a bare, untagged `&T`/`&mut T` return to the
// sole `&'a` parameter's lifetime when exactly one exists.
func (r *Result) resolveReturnType(f FileID, scope ScopeID, id EntityID, selfType TypeID, params []EntityID, n syntax.NodeID) TypeID {
	cand := r.returnElisionCandidate(r.Fn(id), selfType, params)
	t := r.tree(f)
	if cand != 0 && t.Kind(n) == syntax.TypeRef {
		node := t.Nodes[n]
		if t.Toks[node.Tok].Kind != token.Lifetime {
			r.Fn(id).RetLifetime = cand
			elem := r.resolveType(f, scope, syntax.NodeID(node.Rhs), posTypeArg)
			return r.Types.Ref(elem, node.Lhs&syntax.FlagMut != 0)
		}
	}
	ret := r.resolveType(f, scope, n, posReturn)
	r.Fn(id).RetLifetime = r.explicitLifetimeOf(f, scope, n)
	return ret
}

// returnElisionCandidate returns the sole `&'a` source a bare returned
// reference could take, or 0 if there is none or more than one.
func (r *Result) returnElisionCandidate(info *FnInfo, selfType TypeID, params []EntityID) EntityID {
	cand, count := EntityID(0), 0
	if info.Recv != RecvNone && info.Recv != RecvMove {
		var recvLt EntityID
		for _, p := range r.Types.Params(selfType) {
			if !r.typeParam(p).IsLifetime {
				continue
			}
			if recvLt != 0 {
				recvLt = 0
				break
			}
			recvLt = p
		}
		if recvLt != 0 {
			cand, count = recvLt, 1
		}
	}
	for _, p := range params {
		if lt := r.local(p).Lifetime; lt != 0 {
			count++
			cand = lt
		}
	}
	if count == 1 {
		return cand
	}
	return 0
}

// adtArgs instantiates unspecified generic arguments with fresh inference
// variables (for lifetimes too: a spread with its own lifetime
// unifies structurally, like instantiateFn). An untouched lifetime var
// falls back to the function's sole lifetime, or the type's own parameter
// if there is none, so it is at least self-consistent within the literal.
func (c *checker) adtArgs(n syntax.NodeID, adt EntityID, args []TypeID) []TypeID {
	params := c.r.typeDecl(adt).Params
	if len(args) != len(params) {
		args = make([]TypeID, len(params))
		for i := range params {
			if c.r.typeParam(params[i]).IsLifetime {
				fallback := c.r.soleLifetimeParam(c.fn)
				if fallback == 0 {
					fallback = params[i]
				}
				args[i] = c.vars.freshLifetime(n, params[i], fallback)
				continue
			}
			args[i] = c.vars.fresh(n, c.r.Entities[params[i]].Name)
		}
	}
	subst := make(map[EntityID]TypeID, len(params))
	for i, p := range params {
		subst[p] = args[i]
	}
	c.deferConstraints(n, params, args, subst)
	return args
}
