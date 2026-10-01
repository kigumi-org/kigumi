package sem

import "kigumi/internal/syntax"

// checkFieldLifetimeEscape rejects a field type with an unbound lifetime
// parameter not declared by the owner; tag is non-zero only
// for a bare `&'a T` field, already checked by resolveRefType (E1017), so
// this only fires for a named type like `View['a]`.
func (r *Result) checkFieldLifetimeEscape(f FileID, n syntax.NodeID, owner EntityID, typ TypeID, tag EntityID) {
	if tag != 0 || !r.typeHasLifetime(typ) {
		return
	}
	own := map[EntityID]bool{}
	for _, p := range r.typeDecl(owner).Params {
		if r.typeParam(p).IsLifetime {
			own[p] = true
		}
	}
	for _, p := range r.Types.Params(typ) {
		if r.typeParam(p).IsLifetime && !own[p] {
			r.errAt(f, n, cLifetimeEscapes, r.TypeString(typ), "a field without a matching lifetime parameter")
			return
		}
	}
}

// checkLifetimeEscapeArgs rejects a lifetime-carrying type argument to
// `Shared[T]` or `Future[T]`; Shared is matched by name and
// home package rather than Result.langItem, which would misreport
// lang-item-missing for a file that has not imported std/alloc.
func (r *Result) checkLifetimeEscapeArgs(f FileID, n syntax.NodeID, ent EntityID, args []TypeID) {
	alloc, hasAlloc := r.pathIndex["std/alloc"]
	isShared := hasAlloc && r.Entities[ent].Name == "Shared" && r.Entities[ent].Pkg == alloc
	if !isShared && ent != r.Types.FutureEnt() {
		return
	}
	for _, a := range args {
		if r.typeHasLifetime(a) {
			r.errAt(f, n, cLifetimeEscapes, r.TypeString(a), "`"+r.Entities[ent].Name+"`")
		}
	}
}

// typeHasLifetime reports whether t, or a generic argument reachable
// through it, carries a named lifetime.
func (r *Result) typeHasLifetime(t TypeID) bool {
	for _, p := range r.Types.Params(t) {
		if r.typeParam(p).IsLifetime {
			return true
		}
	}
	return false
}
