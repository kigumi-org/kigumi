package sem

import "kigumi/internal/syntax"

// checkVariantPreludeCollision reports a variant name that collides with a
// predeclared, prelude, imported, or package-level type, fn, or const.
func (r *Result) checkVariantPreludeCollision(f FileID, v syntax.NodeID, name string) {
	if k := r.Entities[r.lookupUniverse(name)].Kind; isCollisionEntity(k) {
		r.errAt(f, v, cVariantPreludeCollision, name, "predeclared", collisionNoun(k), name)
		return
	}
	if b, ok := r.Scopes[r.prelude].Names[name]; ok && r.reportScopeCollision(f, v, name, b, "prelude") {
		return
	}
	// resolveImports (pass 2) already resolved every selected import's Target
	// by now, so this sees the real entity, not a pending import.
	if b, ok := r.Scopes[r.fileScopes[f]].Names[name]; ok {
		if target := r.resolveAlias(b.Ent); !r.poisoned(target) {
			if k := r.Entities[target].Kind; isCollisionEntity(k) {
				r.errAt(f, v, cVariantPreludeCollision, name, "imported", collisionNoun(k), name)
				return
			}
		}
	}
	// Variants are never bound in the package scope (declareVariants only
	// records them in r.variantNames), so this can't fire on a sibling
	// ADT's variant; that ambiguity is variantNames' concern, not this one's.
	if b, ok := r.Scopes[r.Packages[r.packageOf(f)].Scope].Names[name]; ok {
		r.reportScopeCollision(f, v, name, b, "declared")
	}
}

// Set is checked first because a fn binds an overload set there (Ent == 0);
// an import's Target is already flattened to a concrete entity.
func (r *Result) reportScopeCollision(f FileID, v syntax.NodeID, name string, b Binding, what string) bool {
	if b.Set != 0 {
		r.errAt(f, v, cVariantPreludeCollision, name, what, collisionNoun(EntFn), name)
		return true
	}
	if !r.poisoned(b.Ent) {
		if k := r.Entities[b.Ent].Kind; isCollisionEntity(k) {
			r.errAt(f, v, cVariantPreludeCollision, name, what, collisionNoun(k), name)
			return true
		}
	}
	return false
}

func isTypeLikeEntity(k EntityKind) bool {
	return k == EntType || k == EntAlias || k == EntInterface
}

func isCollisionEntity(k EntityKind) bool {
	return isTypeLikeEntity(k) || k == EntVariant || k == EntFn || k == EntConst
}

func collisionNoun(k EntityKind) string {
	switch k {
	case EntVariant:
		return "variant"
	case EntFn:
		return "function"
	case EntConst:
		return "const"
	default:
		return "type"
	}
}
