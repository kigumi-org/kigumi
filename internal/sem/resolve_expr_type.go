package sem

import "kigumi/internal/syntax"

// reinterprets an expression node as a type reference.
func (r *Result) exprAsTypeRef(f FileID, scope ScopeID, n syntax.NodeID) (EntityID, []TypeID, bool) {
	t := r.tree(f)
	node := t.Nodes[n]
	switch node.Kind {
	case syntax.Ident:
		b, _, ok := r.lookup(scope, t.TokText(node.Tok))
		if !ok || b.Ent == 0 {
			r.errAt(f, n, cUnresolvedName, t.TokText(node.Tok))
			return 0, nil, false
		}
		ent := r.follow(b.Ent)
		r.Files[f].Uses[n] = ent
		return ent, nil, true
	case syntax.MemberExpr:
		base := t.Nodes[node.Lhs]
		if base.Kind != syntax.Ident {
			break
		}
		b, _, ok := r.lookup(scope, t.TokText(base.Tok))
		if !ok || b.Ent == 0 || r.Entities[b.Ent].Kind != EntImport || r.Entities[b.Ent].Target == 0 ||
			r.Entities[r.Entities[b.Ent].Target].Kind != EntPackage {
			break
		}
		r.Entities[b.Ent].Flags |= EfUsed
		pkg := r.Entities[r.Entities[b.Ent].Target].Pkg
		member, ok := r.Scopes[r.Packages[pkg].Scope].Names[t.TokText(node.Tok)]
		if !ok || member.Ent == 0 {
			r.errAt(f, n, cUnresolvedMember, r.Packages[pkg].Path, t.TokText(node.Tok))
			return 0, nil, false
		}
		ent := r.follow(member.Ent)
		if !r.useEntity(f, n, ent) {
			return 0, nil, false
		}
		r.Files[f].Uses[n] = ent
		return ent, nil, true
	case syntax.BracketExpr:
		ent, _, ok := r.exprAsTypeRef(f, scope, syntax.NodeID(node.Lhs))
		if !ok {
			return 0, nil, false
		}
		var args []TypeID
		for _, a := range t.Children(syntax.NodeID(node.Rhs)) {
			args = append(args, r.exprAsType(f, scope, a))
		}
		return ent, args, true
	}
	r.errAt(f, n, cNotAType, "this", "an expression")
	return 0, nil, false
}

// `T?` parses as TryExpr and becomes Option[T].
func (r *Result) exprAsType(f FileID, scope ScopeID, n syntax.NodeID) TypeID {
	t := r.tree(f)
	node := t.Nodes[n]
	switch node.Kind {
	case syntax.TryExpr:
		return r.Types.Option(r.exprAsType(f, scope, syntax.NodeID(node.Lhs)))
	case syntax.Paren:
		return r.exprAsType(f, scope, syntax.NodeID(node.Lhs))
	case syntax.TypeFn, syntax.TypePtr:
		return r.resolveType(f, scope, n, posTypeArg)
	case syntax.TupleLit:
		return r.exprAsTupleType(f, scope, n)
	}
	ent, args, ok := r.exprAsTypeRef(f, scope, n)
	if !ok {
		return TyPoison
	}
	ty := r.instantiateTypeEntity(f, n, ent, args, posTypeArg)
	r.Files[f].Types[n] = ty
	return ty
}

// resolveExprGenericArg is exprAsType's counterpart to resolveGenericArg: a call type argument
// filling a const parameter (e.g. `zeroed[u8, 32]()`) gets const evaluation instead of an
// ordinary type.
func (r *Result) resolveExprGenericArg(f FileID, scope ScopeID, n syntax.NodeID, param EntityID) TypeID {
	if param != 0 {
		if info := r.typeParam(param); info.IsConst {
			return r.resolveConstArg(f, scope, n, info.ConstType)
		}
	}
	return r.exprAsType(f, scope, n)
}

// exprAsTupleType reinterprets a tuple literal `(a, b)` in a type-argument position as `(A, B)`,
// since a bare `(` can't be told from TypeTuple in the parser.
func (r *Result) exprAsTupleType(f FileID, scope ScopeID, n syntax.NodeID) TypeID {
	items := r.tree(f).Children(n)
	ent, ok := r.Types.tupleEntity(len(items))
	if !ok {
		r.errAt(f, n, cTupleArity, len(items))
		for _, it := range items {
			r.exprAsType(f, scope, it)
		}
		return TyPoison
	}
	args := make([]TypeID, len(items))
	for i, it := range items {
		args[i] = r.exprAsType(f, scope, it)
	}
	ty := r.Types.Named(ent, args)
	r.Files[f].Types[n] = ty
	return ty
}
