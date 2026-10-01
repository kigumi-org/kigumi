package sem

import (
	"slices"

	"kigumi/internal/syntax"
)

// checkVariantEntryCollision reports an entry-package variant named like an
// entry-only intrinsic. Variants are never scope-bound, so
// nothing else would catch entryScope's binding taking over the name.
func (r *Result) checkVariantEntryCollision(f FileID, v syntax.NodeID, name string) {
	pkg := r.packageOf(f)
	if r.Packages[pkg].Entry != 0 && slices.Contains(r.entryOnlyNames(), name) {
		r.errAt(f, v, cEntryNameRedeclared, name)
	}
}
