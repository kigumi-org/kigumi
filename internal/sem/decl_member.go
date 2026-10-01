package sem

import "kigumi/internal/syntax"

// resolveReceiver resolves `fn T[..].name` to its owning nominal type,
// returning 0 and TyPoison on failure.
func (r *Result) resolveReceiver(id EntityID, recv syntax.NodeID, scope ScopeID) (EntityID, TypeID) {
	e := &r.Entities[id]
	f := e.File
	t := r.tree(f)
	node := t.Nodes[recv]
	path := syntax.NodeID(node.Lhs)
	name := pathText(t, path, ".")
	b, _, ok := r.lookup(scope, name)
	if !ok || b.Ent == 0 {
		r.errAt(f, recv, cMemberNotNominal, name)
		return 0, TyPoison
	}
	owner := r.follow(b.Ent)
	oe := &r.Entities[owner]
	r.Files[f].Uses[recv] = owner
	switch oe.Kind {
	case EntAlias:
		r.errAt(f, recv, cAliasReceiver, name)
		return 0, TyPoison
	case EntTypeParam:
		r.errAt(f, recv, cRecvTypeParam)
		return 0, TyPoison
	case EntType:
	default:
		r.errAt(f, recv, cMemberNotNominal, name)
		return 0, TyPoison
	}
	info := r.typeDecl(owner)
	// The orphan rule is authorized later, in authorizeForeignMember
	// (called from declareFn), once Self substitution has the receiver's
	// own type parameters resolved below.
	var binders []syntax.NodeID
	if node.Rhs != 0 {
		binders = t.Children(syntax.NodeID(node.Rhs))
	}
	if len(info.Params) > 0 && len(binders) == 0 {
		// Associated functions may omit the binder; methods may not,
		// which declareFn reports once it knows there is a `self`.
		r.Fn(id).Owner = owner
		return owner, TyPoison
	}
	if len(binders) != len(info.Params) {
		r.errAt(f, recv, cRecvBinderMismatch, r.paramNames(info.Params), name)
		return owner, TyPoison
	}
	var args []TypeID
	bounded := false
	for _, bn := range binders {
		if t.Kind(bn) == syntax.GenericParam && t.Nodes[bn].Rhs != 0 {
			bounded = true
		}
	}
	var params []EntityID
	for i, bn := range binders {
		want := r.Entities[info.Params[i]].Name
		got := ""
		switch t.Kind(bn) {
		case syntax.TypePath:
			got = pathText(t, syntax.NodeID(t.Nodes[bn].Lhs), ".")
		case syntax.GenericParam:
			got = t.TokText(t.Nodes[bn].Tok)
		}
		if got != want {
			r.errAt(f, recv, cRecvBinderMismatch, r.paramNames(info.Params), name)
			return owner, TyPoison
		}
		param := info.Params[i]
		if bounded {
			// A bound on a binder makes the method generic in its own right,
			// so it gets fresh parameters distinct from the owner's.
			param = r.newEntity(Entity{Kind: EntTypeParam, Name: want, Pkg: e.Pkg, File: f, Node: bn, Tok: t.Nodes[bn].Tok, Parent: id, Vis: Visibility{Level: VisPub}})
			ownInfo := r.typeParam(info.Params[i])
			r.Entities[param].Detail = r.addTypeParam(TypeParamInfo{Index: i, Constraints: append([]Constraint(nil), ownInfo.Constraints...), IsConst: ownInfo.IsConst, ConstType: ownInfo.ConstType, IsLifetime: ownInfo.IsLifetime})
			r.Entities[param].Type = r.Types.Param(param)
			r.Files[f].Defs[bn] = param
		}
		r.define(scope, want, Binding{Ent: param})
		args = append(args, r.Types.Param(param))
		params = append(params, param)
	}
	if bounded {
		r.resolveConstraints(f, scope, syntax.NodeID(node.Rhs), params)
		r.Fn(id).RecvParams = params
	}
	r.Fn(id).TypeParams = append([]EntityID(nil), params...)
	if oe.Type != 0 && len(info.Params) == 0 {
		return owner, oe.Type
	}
	return owner, r.Types.Named(owner, args)
}
