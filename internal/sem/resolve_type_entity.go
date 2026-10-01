package sem

import "kigumi/internal/syntax"

func (r *Result) resolveTypePath(f FileID, scope ScopeID, n syntax.NodeID, pos typePos) TypeID {
	t := r.tree(f)
	node := t.Nodes[n]
	path := syntax.NodeID(node.Lhs)
	ent := r.resolveTypeName(f, scope, path)
	if ent == 0 {
		return TyPoison
	}
	r.Files[f].Uses[path] = ent
	r.Files[f].Uses[n] = ent
	var args []TypeID
	if node.Rhs != 0 {
		params := r.genericParamsOf(ent)
		for i, a := range t.Children(syntax.NodeID(node.Rhs)) {
			args = append(args, r.resolveGenericArg(f, scope, a, entAt(params, i)))
		}
	}
	r.checkLifetimeEscapeArgs(f, n, ent, args)
	return r.instantiateTypeEntity(f, n, ent, args, pos)
}

func (r *Result) genericParamsOf(ent EntityID) []EntityID {
	switch r.Entities[ent].Kind {
	case EntType:
		return r.typeDecl(ent).Params
	case EntAlias:
		return r.aliasParams[ent]
	case EntInterface:
		return r.iface(ent).Params
	}
	return nil
}

// param is 0 when unknown, e.g. an arity mismatch.
func (r *Result) resolveGenericArg(f FileID, scope ScopeID, n syntax.NodeID, param EntityID) TypeID {
	if param != 0 {
		info := r.typeParam(param)
		switch {
		case info.IsConst:
			return r.resolveConstArg(f, scope, n, info.ConstType)
		case info.IsLifetime:
			return r.resolveLifetimeArg(f, scope, n)
		}
	}
	return r.resolveType(f, scope, n, posTypeArg)
}

func (r *Result) instantiateTypeEntity(f FileID, n syntax.NodeID, ent EntityID, args []TypeID, pos typePos) TypeID {
	e := &r.Entities[ent]
	switch e.Kind {
	case EntAlias:
		params := r.aliasParams[ent]
		if !r.checkArity(f, n, ent, len(params), len(args)) {
			return TyPoison
		}
		target := r.aliasType(ent)
		if len(params) == 0 {
			return target
		}
		subst := map[EntityID]TypeID{}
		for i, p := range params {
			subst[p] = args[i]
		}
		return r.Types.Subst(target, subst)
	case EntTypeParam:
		if len(args) > 0 {
			r.errAt(f, n, cTypeNotGeneric, e.Name)
		}
		return r.Types.Param(ent)
	case EntConstraint:
		r.errAt(f, n, cConstraintAsType, e.Name)
		return TyPoison
	case EntInterface:
		if !r.checkArity(f, n, ent, len(r.iface(ent).Params), len(args)) {
			return TyPoison
		}
		if pos != posConstraint && !r.objectSafe(ent) {
			r.errAt(f, n, cIfaceNotObjectSafe, e.Name, r.iface(ent).ObjectSafeWhy)
		}
		return r.Types.Iface(ent, args)
	case EntType:
		info := r.typeDecl(ent)
		if e.Type != 0 && len(info.Params) == 0 {
			// Both backends hold integers in 64 bits; refused
			// here rather than truncated downstream.
			if e.Type == TyI128 || e.Type == TyU128 {
				r.errAt(f, n, cInt128Unsupported, e.Name)
				return TyPoison
			}
			if len(args) > 0 {
				r.errAt(f, n, cTypeNotGeneric, e.Name)
			}
			return e.Type
		}
		if !r.checkArity(f, n, ent, len(info.Params), len(args)) {
			return TyPoison
		}
		return r.Types.Named(ent, args)
	}
	r.errAt(f, n, cNotAType, e.Name, "a "+e.Kind.String())
	return TyPoison
}

func (r *Result) checkArity(f FileID, n syntax.NodeID, ent EntityID, want, got int) bool {
	switch {
	case want == got:
		return true
	case got == 0:
		r.errAt(f, n, cTypeArgsMissing, r.Entities[ent].Name, r.Entities[ent].Name)
	case want == 0:
		r.errAt(f, n, cTypeNotGeneric, r.Entities[ent].Name)
	default:
		r.errAt(f, n, cTypeArgCount, r.Entities[ent].Name, want, plural(want), got)
	}
	return false
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
