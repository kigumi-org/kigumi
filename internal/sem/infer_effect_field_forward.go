package sem

import "kigumi/internal/syntax"

// fieldForwardEdge: case-(b) forwarding relation.
type fieldForwardEdge struct {
	Caller, CallerParam, Callee, CalleeParam EntityID
}

// closeFieldParamForwarding pre-seeds CalledFieldParams to a fixpoint over
// the whole call graph: declaration order alone would miss a `pure?` field
// forwarded through more than one function before its callee is checked.
func (r *Result) closeFieldParamForwarding() {
	var edges []fieldForwardEdge
	for id := 1; id < len(r.Entities); id++ {
		e := &r.Entities[id]
		if e.Kind != EntFn || e.Flags&EfPoison != 0 || e.File == 0 {
			continue
		}
		info := r.Fn(EntityID(id))
		if info.Body == 0 || r.Entities[e.Parent].Kind == EntInterface {
			continue
		}
		edges = append(edges, r.scanFieldForwarding(EntityID(id), e.File, info)...)
	}
	for changed := true; changed; {
		changed = false
		for _, fe := range edges {
			for _, key := range r.Fn(fe.Callee).CalledFieldParams {
				if key.Param == fe.CalleeParam && r.addCalledFieldParam(fe.Caller, fe.CallerParam, key.Field) {
					changed = true
				}
			}
		}
	}
}

func (r *Result) addCalledFieldParam(fn, param, field EntityID) bool {
	info := r.Fn(fn)
	key := FieldParamKey{Param: param, Field: field}
	for _, k := range info.CalledFieldParams {
		if k == key {
			return false
		}
	}
	info.CalledFieldParams = append(info.CalledFieldParams, key)
	return true
}

func (r *Result) scanFieldForwarding(fn EntityID, file FileID, info *FnInfo) []fieldForwardEdge {
	var edges []fieldForwardEdge
	t := r.tree(file)
	scope := r.declScope(fn)
	var walk func(n syntax.NodeID)
	walk = func(n syntax.NodeID) {
		if n == 0 || t.Kind(n) == syntax.Lambda {
			return
		}
		if t.Kind(n) == syntax.CallExpr || t.Kind(n) == syntax.WsCallExpr {
			r.scanFieldCall(fn, info, t, n, scope, &edges)
		}
		t.EachChild(n, walk)
	}
	walk(info.Body)
	return edges
}

func (r *Result) scanFieldCall(fn EntityID, info *FnInfo, t *syntax.Tree, call syntax.NodeID, scope ScopeID, edges *[]fieldForwardEdge) {
	node := t.Nodes[call]
	callee := syntax.NodeID(node.Lhs)
	if t.Kind(callee) == syntax.MemberExpr {
		mnode := t.Nodes[callee]
		if p := r.paramIdent(info, t, syntax.NodeID(mnode.Lhs)); p != 0 {
			if fld := r.effectPolyFieldOn(r.Entities[p].Type, t.TokText(mnode.Tok)); fld != 0 {
				r.addCalledFieldParam(fn, p, fld)
			}
		}
		return
	}
	if t.Kind(callee) != syntax.Ident {
		return
	}
	args := t.Children(syntax.NodeID(node.Rhs))
	for _, g := range r.candidateFnBindings(scope, t.TokText(t.Nodes[callee].Tok)) {
		gParams := r.Fn(g).Params
		if r.callArgsDefinitelyMismatch(info, t, args, gParams) {
			continue
		}
		for i, a := range args {
			if i >= len(gParams) || t.Kind(a) == syntax.Spread {
				continue
			}
			if p := r.paramIdent(info, t, a); p != 0 {
				*edges = append(*edges, fieldForwardEdge{Caller: fn, CallerParam: p, Callee: g, CalleeParam: gParams[i]})
			}
		}
	}
}

// paramIdent matches by text instead of real scope tracking: a wrong extra
// key only makes a caller's forwarding check more conservative, never less
// sound.
func (r *Result) paramIdent(info *FnInfo, t *syntax.Tree, n syntax.NodeID) EntityID {
	if t.Kind(n) != syntax.Ident {
		return 0
	}
	name := t.TokText(t.Nodes[n].Tok)
	for _, p := range info.Params {
		if r.Entities[p].Name == name {
			if r.Entities[p].Flags&EfMut != 0 {
				return 0
			}
			return p
		}
	}
	return 0
}

func (r *Result) effectPolyFieldOn(typ TypeID, name string) EntityID {
	if r.Types.Kind(typ) == KRef {
		typ = r.Types.Node(typ).Elem
	}
	if r.Types.Kind(typ) != KNamed {
		return 0
	}
	if fld := r.findField(r.Types.Node(typ).Ent, name); fld != 0 && r.isEffectPolyField(fld) {
		return fld
	}
	return 0
}

// candidateFnBindings over-approximates: this pass runs before
// body-checking, so it can't run pickOverload's full resolution. A wrong
// candidate only makes the caller's forwarding check more conservative,
// never less sound, same as an extra CalledFieldParams key (see paramIdent).
func (r *Result) candidateFnBindings(scope ScopeID, name string) []EntityID {
	b, _, ok := r.lookup(scope, name)
	if !ok {
		return nil
	}
	ent := b.Ent
	if ent == 0 {
		return r.overloadFnMembers(b.Set)
	}
	ent = r.follow(ent)
	if r.Entities[ent].Kind != EntFn {
		return nil
	}
	if set := r.Fn(ent).Set; set != 0 && len(r.Overloads[set].Members) > 1 {
		return r.overloadFnMembers(set)
	}
	return []EntityID{ent}
}

func (r *Result) overloadFnMembers(set OverloadSetID) []EntityID {
	var out []EntityID
	for _, m := range r.Overloads[set].Members {
		if r.Entities[m].Kind == EntFn {
			out = append(out, m)
		}
	}
	return out
}
