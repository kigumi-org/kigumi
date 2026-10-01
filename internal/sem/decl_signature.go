package sem

import (
	"strings"

	"kigumi/internal/syntax"
)

// declareSignatures runs as pass 4.
func (r *Result) declareSignatures() {
	var fns []EntityID
	for id := 1; id < len(r.Entities); id++ {
		e := &r.Entities[id]
		if e.Kind == EntFn && e.File != 0 && e.Flags&EfPoison == 0 && r.Entities[e.Parent].Kind != EntInterface {
			fns = append(fns, EntityID(id))
		}
	}
	for _, id := range fns {
		r.declareFn(id)
	}
	for _, id := range fns {
		if r.Fn(id).Owner != 0 {
			r.registerMember(id)
		}
	}
	r.checkOverloadSets()
	r.declareAttrs()
	r.checkDeclarations()
}

func (r *Result) declareFn(id EntityID) {
	e := &r.Entities[id]
	f := e.File
	t := r.tree(f)
	s := fnDecl(t, e.Node)
	scope := r.declScope(id)
	info := r.Fn(id)
	var selfType TypeID
	if s.Recv != 0 {
		owner, ownerType := r.resolveReceiver(id, s.Recv, scope)
		info = r.Fn(id)
		info.Owner = owner
		selfType = ownerType
	}
	own := r.declareGenerics(f, id, s.Generics, scope)
	r.resolveConstraints(f, scope, s.Generics, own)
	r.Fn(id).TypeParams = append(r.Fn(id).TypeParams, own...)
	r.resolveSignature(id, scope, selfType)
	if s.Recv != 0 {
		if owner := r.Fn(id).Owner; owner != 0 {
			r.authorizeForeignMember(id, owner, selfType, s.Recv)
		}
	}
	r.declareWitnesses(id)
	r.declareConstParams(id)
	info = r.Fn(id)
	if (info.Abi != "" || info.Export != "") && len(r.constParamsOf(id)) > 0 {
		r.errAt(f, e.Node, cExportConstParam, e.Name)
	}
	if s.Ret == 0 {
		r.errAt(f, e.Node, cFnReturnTypeRequired, e.Name)
	}
	mods := Effects(s.Mods)
	switch {
	case mods&EffAsync != 0 && mods&EffPure != 0:
		r.errAt(f, e.Node, cModsAsyncConflict, "pure")
	case mods&EffAsync != 0 && mods&EffUnsafe != 0:
		r.errAt(f, e.Node, cModsAsyncConflict, "unsafe")
	case mods&EffAsync != 0 && mods&EffNoalloc != 0:
		r.errAt(f, e.Node, cUnsupportedAsync)
	}
	if info.Abi != "" && mods&(EffPure|EffNoalloc|EffAsync) != 0 {
		r.errAt(f, e.Node, cForeignMods, effectsText(mods&(EffPure|EffNoalloc|EffAsync)))
	}
	if info.Naked && mods != 0 {
		r.errAt(f, e.Node, cNakedMods, effectsText(mods))
	}
	if info.Naked && s.Recv != 0 {
		r.errAt(f, e.Node, cNakedMethod)
	}
	if s.Mods&syntax.ModPureVar != 0 {
		r.errAt(f, e.Node, cEffectPolyNotReq)
	}
	if info.Abi != "" || info.Export != "" {
		for _, prefix := range []string{"kg_export_", "kg_byval_", "kg_call_"} {
			if strings.HasPrefix(e.Name, prefix) {
				r.errAt(f, e.Node, cCSymbolReserved, e.Name)
			}
		}
	}
	if info.Body == 0 && info.Abi == "" && e.Flags&EfStd == 0 {
		r.errAt(f, e.Node, cFnMissingBody, e.Name)
	}
	if info.Body != 0 && info.Abi != "" {
		r.errAt(f, e.Node, cExternBody, e.Name)
	}
	if info.Owner == 0 && info.SelfParam != 0 && s.Recv == 0 {
		r.errAt(f, r.Entities[info.SelfParam].Node, cSelfOutsideMember)
		info.Recv = RecvNone
	}
	if info.Owner != 0 && info.SelfParam != 0 && t.Nodes[s.Recv].Rhs == 0 && len(r.typeDecl(info.Owner).Params) > 0 {
		owner := r.Entities[info.Owner].Name
		r.errAt(f, s.Recv, cRecvGenericBinder, owner, owner, r.paramNames(r.typeDecl(info.Owner).Params), e.Name)
	}
}

// resolveSignature creates a function or interface requirement's parameter
// entities and KFn signature; selfType is 0 for free functions.
func (r *Result) resolveSignature(id EntityID, scope ScopeID, selfType TypeID) {
	e := &r.Entities[id]
	f := e.File
	t := r.tree(f)
	s := fnDecl(t, e.Node)
	info := r.Fn(id)
	var params []TypeID
	variadic := false
	cvariadic := false
	list := t.Children(s.Params)
	for i, p := range list {
		ps := param(t, p)
		name := t.TokText(t.Nodes[p].Tok)
		pe := Entity{Kind: EntParam, Name: name, Pkg: e.Pkg, File: f, Node: p, Tok: t.Nodes[p].Tok, Parent: id}
		if ps.Flags&syntax.FlagSelf != 0 {
			if i != 0 && s.Recv != 0 {
				r.errAt(f, p, cSelfPosition)
			}
			pe.Flags |= EfSelf
			switch {
			case ps.Flags&syntax.FlagMove != 0:
				pe.Flags |= EfMove
				info.Recv = RecvMove
			case ps.Flags&syntax.FlagMut != 0:
				pe.Flags |= EfMut
				info.Recv = RecvMut
			default:
				info.Recv = RecvSelf
			}
			pe.Type = selfType
			if selfType == 0 {
				pe.Type = TyPoison
			}
			sid := r.newEntity(pe)
			r.Entities[sid].Detail = r.addLocal(LocalInfo{Scope: scope})
			r.Files[f].Defs[p] = sid
			r.define(scope, "self", Binding{Ent: sid})
			info.SelfParam = sid
			continue
		}
		if ps.Flags&syntax.FlagVariadic != 0 && ps.Type == 0 {
			if info.Abi == "" {
				r.errAt(f, p, cCVariadicOutsideExtern)
			}
			cvariadic = true
			continue
		}
		typ := r.resolveType(f, scope, ps.Type, posParam)
		if ps.Flags&syntax.FlagVariadic != 0 {
			if i != len(list)-1 {
				r.errAt(f, p, cVariadicNotLast)
			}
			variadic = true
			pe.Flags |= EfVariadic
			typ = r.Types.Named(r.Types.arrayEnt, []TypeID{typ})
		}
		if ps.Flags&syntax.FlagMut != 0 {
			pe.Flags |= EfMut
		}
		if name == "nil" {
			r.errAt(f, p, cNilName)
		}
		pe.Type = typ
		pid := r.newEntity(pe)
		r.Entities[pid].Detail = r.addLocal(LocalInfo{Scope: scope, Lifetime: r.explicitLifetimeOf(f, scope, ps.Type)})
		r.Files[f].Defs[p] = pid
		if _, dup := r.define(scope, name, Binding{Ent: pid}); dup {
			r.errAt(f, p, cParamDuplicate, name)
		}
		info.Params = append(info.Params, pid)
		params = append(params, typ)
	}
	ret := TyPoison
	if s.Ret != 0 {
		ret = r.resolveReturnType(f, scope, id, selfType, info.Params, s.Ret)
	}
	info = r.Fn(id)
	switch {
	case info.Abi != "" || info.Naked:
		info.Sig = r.Types.FnCPtr(params, ret, cvariadic)
	case info.EffectPoly:
		info.Sig = r.Types.FnEffectPoly(params, ret, info.Declared&(EffPure|EffNoalloc|EffUnsafe), variadic)
	default:
		info.Sig = r.Types.Fn(params, ret, info.Declared&(EffPure|EffNoalloc|EffUnsafe), variadic)
	}
	r.Entities[id].Type = info.Sig
	if info.Abi != "" || info.Export != "" {
		r.checkAbiTypes(f, s, params, ret)
	}
}
