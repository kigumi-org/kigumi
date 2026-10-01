package sem

import (
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// resolveConstLit turns a literal reached via the type grammar (e.g. `FixedArray[u8, 32]`), not
// a call's explicit type args, into a KConst value.
func (r *Result) resolveConstLit(f FileID, n syntax.NodeID) TypeID {
	t := r.tree(f)
	node := t.Nodes[n]
	if node.Kind == syntax.BoolLit {
		v := int64(0)
		if t.Toks[node.Tok].Kind == token.KwTrue {
			v = 1
		}
		return r.Types.ConstVal(v, TyBool)
	}
	v, ok := syntax.ParseInt(t.TokText(node.Tok))
	if !ok || !v.IsInt64() {
		r.errAt(f, n, cLiteralOutOfRange, t.TokText(node.Tok), TyUsize)
		return TyPoison
	}
	return r.Types.ConstVal(v.Int64(), TyUsize)
}

// `N + 1`-shaped expressions are out of scope for this phase.
func (r *Result) resolveConstArg(f FileID, scope ScopeID, n syntax.NodeID, want TypeID) TypeID {
	t := r.tree(f)
	switch t.Kind(n) {
	case syntax.Paren:
		return r.resolveConstArg(f, scope, syntax.NodeID(t.Nodes[n].Lhs), want)
	case syntax.IntLit, syntax.BoolLit:
		return r.coerceConstLit(f, n, want)
	case syntax.Unary:
		node := t.Nodes[n]
		lhs := syntax.NodeID(node.Lhs)
		if t.Toks[node.Tok].Kind == token.Minus && t.Kind(lhs) == syntax.IntLit {
			return r.coerceNegConstLit(f, n, lhs, want)
		}
		r.errAt(f, n, cConstExprUnsupported)
		return TyPoison
	case syntax.Ident:
		return r.constArgFromIdent(f, scope, n, n, want)
	case syntax.TypePath:
		node := t.Nodes[n]
		if node.Rhs != 0 {
			r.errAt(f, n, cConstExprUnsupported)
			return TyPoison
		}
		return r.constArgFromIdent(f, scope, syntax.NodeID(node.Lhs), n, want)
	}
	r.errAt(f, n, cConstExprUnsupported)
	return TyPoison
}

func (r *Result) constArgFromIdent(f FileID, scope ScopeID, path, n syntax.NodeID, want TypeID) TypeID {
	t := r.tree(f)
	var ent EntityID
	if t.Kind(path) == syntax.Ident {
		b, _, ok := r.lookup(scope, t.TokText(t.Nodes[path].Tok))
		if !ok || b.Ent == 0 {
			r.errAt(f, n, cUnresolvedName, t.TokText(t.Nodes[path].Tok))
			return TyPoison
		}
		ent = r.follow(b.Ent)
	} else {
		ent = r.resolveTypeName(f, scope, path)
		if ent == 0 {
			return TyPoison
		}
	}
	r.Files[f].Uses[n] = ent
	switch r.Entities[ent].Kind {
	case EntTypeParam:
		info := r.typeParam(ent)
		if !info.IsConst {
			r.errAt(f, n, cConstArgKind, r.Entities[ent].Name, "a const", "a type")
			return TyPoison
		}
		if want != 0 && info.ConstType != want {
			r.errAt(f, n, cTypeMismatch, want, info.ConstType)
			return TyPoison
		}
		return r.Types.Param(ent)
	case EntConst:
		v := r.evalConstDecl(ent)
		if v.Kind != constInt && v.Kind != constBool {
			r.errAt(f, n, cConstExprUnsupported)
			return TyPoison
		}
		return r.coerceConstValue(f, n, v, want)
	}
	r.errAt(f, n, cConstArgKind, r.Entities[ent].Name, "a const", "a type")
	return TyPoison
}
