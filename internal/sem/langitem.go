package sem

import "kigumi/internal/syntax"

// Lang items are compiler-known names that live in std packages. They
// resolve lazily; a missing package is reported once.
var langItemTable = map[string]string{
	"Range":          "std/prelude",
	"Branch":         "std/prelude",
	"Fallback":       "std/prelude",
	"Ordering":       "std/prelude",
	"Eq":             "std/prelude",
	"Hash":           "std/prelude",
	"Ord":            "std/prelude",
	"Plan":           "std/shell",
	"Arg":            "std/shell",
	"AllocatorScope": "std/alloc",
	"Host":           "std/os",
	"Context":        "std/error",
	"FixedArray":     "std/prelude",
}

// langItem returns the entity of a lang item, or 0 (and reports
// lang-item-missing at node the first time).
func (r *Result) langItem(f FileID, node syntax.NodeID, name string) EntityID {
	if id, ok := r.langItems[name]; ok {
		return id
	}
	pkgPath, known := langItemTable[name]
	if known {
		if pkg, ok := r.pathIndex[pkgPath]; ok {
			if b, ok := r.Scopes[r.Packages[pkg].Scope].Names[name]; ok && b.Ent != 0 {
				r.langItems[name] = b.Ent
				return b.Ent
			}
		}
	}
	if !r.langItemMissing[name] {
		r.langItemMissing[name] = true
		if f != 0 {
			r.errAt(f, node, cLangItemMissing, name, pkgPath)
		}
	}
	return 0
}
