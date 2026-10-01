package sem

import "kigumi/internal/syntax"

// declareGenerics defers constraints to resolveConstraints afterwards so
// later parameters may appear in earlier bounds.
func (r *Result) declareGenerics(f FileID, owner EntityID, list syntax.NodeID, scope ScopeID) []EntityID {
	if list == 0 {
		return nil
	}
	t := r.tree(f)
	var params []EntityID
	for i, g := range t.Children(list) {
		name := t.TokText(t.Nodes[g].Tok)
		id := r.newEntity(Entity{Kind: EntTypeParam, Name: name, Pkg: r.packageOf(f), File: f, Node: g, Tok: t.Nodes[g].Tok, Parent: owner, Vis: Visibility{Level: VisPub}})
		r.Entities[id].Detail = r.addTypeParam(TypeParamInfo{Index: i})
		r.Entities[id].Type = r.Types.Param(id)
		r.Files[f].Defs[g] = id
		if name == "nil" {
			r.errAt(f, g, cNilName)
		}
		if _, dup := r.define(scope, name, Binding{Ent: id}); dup {
			r.errAt(f, g, cGenericDuplicate, name)
		}
		if t.Nodes[g].Lhs&syntax.FlagConst != 0 {
			ct := r.resolveType(f, scope, syntax.NodeID(t.Nodes[g].Rhs), posConst)
			if ct != TyPoison && !r.allowedConstType(ct) {
				r.errAt(f, g, cConstParamType, r.TypeString(ct))
				ct = TyPoison
			}
			info := r.typeParam(id)
			info.IsConst, info.ConstType = true, ct
		}
		if t.Nodes[g].Lhs&syntax.FlagLifetime != 0 {
			r.typeParam(id).IsLifetime = true
		}
		params = append(params, id)
	}
	return params
}

func (r *Result) resolveConstraints(f FileID, scope ScopeID, list syntax.NodeID, params []EntityID) {
	if list == 0 {
		return
	}
	t := r.tree(f)
	for i, g := range t.Children(list) {
		if i >= len(params) || r.typeParam(params[i]).IsConst {
			continue
		}
		bounds := syntax.NodeID(t.Nodes[g].Rhs)
		if bounds == 0 {
			continue
		}
		info := r.typeParam(params[i])
		for _, b := range t.Children(bounds) {
			if c, ok := r.resolveConstraint(f, scope, b); ok {
				info.Constraints = append(info.Constraints, c)
			}
		}
	}
}

func (r *Result) resolveConstraint(f FileID, scope ScopeID, n syntax.NodeID) (Constraint, bool) {
	t := r.tree(f)
	if t.Kind(n) != syntax.TypePath {
		r.errAt(f, n, cConstraintNotIface, "this")
		return Constraint{}, false
	}
	node := t.Nodes[n]
	ent := r.resolveTypeName(f, scope, syntax.NodeID(node.Lhs))
	if ent == 0 {
		return Constraint{}, false
	}
	r.Files[f].Uses[n] = ent
	e := &r.Entities[ent]
	switch e.Kind {
	case EntInterface:
		ty := r.resolveType(f, scope, n, posConstraint)
		if ty == TyPoison {
			return Constraint{}, false
		}
		return Constraint{Kind: CIface, Type: ty, Node: n}, true
	case EntConstraint:
		switch e.Name {
		case "Copy":
			return Constraint{Kind: CCopy, Node: n}, true
		case "Send":
			return Constraint{Kind: CSend, Node: n}, true
		case "Sync":
			return Constraint{Kind: CSync, Node: n}, true
		}
		args := t.Children(syntax.NodeID(node.Rhs))
		if len(args) != 1 || t.Kind(args[0]) != syntax.TypeFn {
			r.errAt(f, n, cConstraintNotIface, e.Name)
			return Constraint{}, false
		}
		sig := r.resolveType(f, scope, args[0], posTypeArg)
		kind := map[string]ConstraintKind{"Fn": CFn, "FnMut": CFnMut, "FnOnce": CFnOnce}[e.Name]
		return Constraint{Kind: kind, Type: sig, Node: n}, true
	}
	r.errAt(f, n, cConstraintNotIface, e.Name)
	return Constraint{}, false
}

func (r *Result) declScope(ent EntityID) ScopeID {
	if s, ok := r.declScopes[ent]; ok {
		return s
	}
	e := &r.Entities[ent]
	s := r.newScope(ScopeFn, r.fileScopes[e.File], e.File, e.Node)
	r.Scopes[s].Fn = ent
	r.declScopes[ent] = s
	return s
}
