package sem

import "kigumi/internal/syntax"

// Borrows are only legal in parameter and local positions.
type typePos uint8

const (
	posParam typePos = iota
	posLocal
	posReturn
	posField
	posPayload
	posTypeArg
	posAlias
	posConstraint
	posConst
)

var typePosNames = [...]string{
	"parameter types", "local types", "return types", "field types", "variant payloads",
	"type arguments", "alias targets", "constraints", "const types",
}

// Failures are reported once and yield TyPoison.
func (r *Result) resolveType(f FileID, scope ScopeID, n syntax.NodeID, pos typePos) TypeID {
	if n == 0 {
		return TyPoison
	}
	t := r.tree(f)
	node := t.Nodes[n]
	var out TypeID
	switch node.Kind {
	case syntax.TypePath:
		out = r.resolveTypePath(f, scope, n, pos)
	case syntax.TypeOptional:
		out = r.Types.Option(r.resolveType(f, scope, syntax.NodeID(node.Lhs), posTypeArg))
	case syntax.TypeResult:
		out = r.Types.Result(r.resolveType(f, scope, syntax.NodeID(node.Lhs), posTypeArg), r.Types.errorType())
	case syntax.TypeFn:
		out = r.resolveFnType(f, scope, n)
	case syntax.TypePtr:
		out = r.Types.Ptr(r.resolveType(f, scope, syntax.NodeID(node.Rhs), posTypeArg), node.Lhs&syntax.FlagMut != 0)
	case syntax.TypeRef:
		out = r.resolveRefType(f, scope, n, node, pos)
	case syntax.Paren:
		out = r.resolveType(f, scope, syntax.NodeID(node.Lhs), pos)
	case syntax.TypeTuple:
		out = r.resolveTupleType(f, scope, n, pos)
	default:
		r.errAt(f, n, cNotAType, "this", "an expression")
		out = TyPoison
	}
	r.Files[f].Types[n] = out
	return out
}

func (r *Result) resolveFnType(f FileID, scope ScopeID, n syntax.NodeID) TypeID {
	t := r.tree(f)
	s := fnType(t, n)
	var params []TypeID
	for _, p := range t.Children(s.Params) {
		params = append(params, r.resolveType(f, scope, p, posParam))
	}
	ret := r.resolveType(f, scope, s.Ret, posReturn)
	if s.Abi != 0 {
		return r.resolveCFnType(f, n, s, params, ret)
	}
	if s.Flags&syntax.FlagVariadic != 0 {
		r.errAt(f, n, cFnTypeVariadic)
	}
	eff := Effects(s.Mods) & (EffPure | EffNoalloc | EffUnsafe)
	if s.Mods&syntax.ModPureVar != 0 {
		return r.Types.FnEffectPoly(params, ret, eff, false)
	}
	return r.Types.Fn(params, ret, eff, false)
}

// extern(C) fn types forbid effect modifiers and require ABI-safe
// parameters and result.
func (r *Result) resolveCFnType(f FileID, n syntax.NodeID, s fnTypeSlots, params []TypeID, ret TypeID) TypeID {
	t := r.tree(f)
	if name := t.TokText(s.Abi); name != "C" {
		r.errAt(f, n, cAbiUnknown, name)
	}
	switch {
	case s.Mods&syntax.ModPureVar != 0:
		r.errAt(f, n, cCFnTypeMods, "pure?")
	case Effects(s.Mods) != 0:
		r.errAt(f, n, cCFnTypeMods, effectsText(Effects(s.Mods)))
	}
	for i, p := range t.Children(s.Params) {
		if !r.abiSafe(params[i]) {
			r.errAt(f, p, cAbiType, r.TypeString(params[i]))
		}
	}
	if ret != TyUnit && !r.abiSafe(ret) {
		r.errAt(f, s.Ret, cAbiType, r.TypeString(ret))
	}
	return r.Types.FnCPtr(params, ret, s.Flags&syntax.FlagVariadic != 0)
}
