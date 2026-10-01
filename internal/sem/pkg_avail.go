package sem

import "kigumi/internal/syntax"

// Layer is a std package's dependency layer: Foundation/Allocation need no
// OS, Platform needs one, Tool is build-sandbox only.
type Layer uint8

const (
	LayerFoundation Layer = iota
	LayerAllocation
	LayerPlatform
	LayerTool
)

// stdLayer is the package-to-layer table. std/random is Platform, not
// Foundation: fromClock() reads std/time.
var stdLayer = map[string]Layer{
	"std/prelude":         LayerFoundation,
	"std/math":            LayerFoundation,
	"std/ffi":             LayerFoundation,
	"std/task":            LayerFoundation,
	"std/test":            LayerFoundation,
	"std/crypto/subtle":   LayerFoundation,
	"std/alloc":           LayerAllocation,
	"std/binary":          LayerAllocation,
	"std/encoding/base64": LayerAllocation,
	"std/encoding/hex":    LayerAllocation,
	"std/fmt":             LayerAllocation,
	"std/error":           LayerAllocation,
	"std/array":           LayerAllocation,
	"std/map":             LayerAllocation,
	"std/text":            LayerAllocation,
	"std/json":            LayerAllocation,
	"std/io":              LayerAllocation,
	"std/dl":              LayerPlatform,
	"std/entropy":         LayerPlatform,
	"std/fs":              LayerPlatform,
	"std/net":             LayerPlatform,
	"std/os":              LayerPlatform,
	"std/random":          LayerPlatform,
	"std/shell":           LayerPlatform,
	"std/shell/coreutil":  LayerPlatform,
	"std/time":            LayerPlatform,
	"std/build":           LayerTool,
}

// Runs before resolveImports binds anything, so the graph is
// order-independent across packages; platformAvailability walks it
// transitively, and this pass also checks layer direction per edge.
func (r *Result) buildStdImportGraph() map[PackageID][]PackageID {
	g := make(map[PackageID][]PackageID, len(r.Packages))
	for i := 1; i < len(r.Packages); i++ {
		id := PackageID(i)
		if !r.Packages[id].Std {
			continue
		}
		fromLayer, hasFrom := stdLayer[r.Packages[id].Path]
		for _, f := range r.Packages[id].Files {
			t := r.tree(f)
			for _, d := range r.topDecls(f) {
				if t.Kind(d) != syntax.ImportDecl {
					continue
				}
				s := importDecl(t, d)
				path := pathText(t, s.Path, "/")
				target, ok := r.pathIndex[path]
				if !ok || !r.Packages[target].Std {
					continue
				}
				g[id] = append(g[id], target)
				if toLayer, hasTo := stdLayer[path]; hasFrom && hasTo && toLayer > fromLayer {
					r.errAt(f, s.Path, cStdLayerDirection, path, r.Packages[id].Path)
				}
			}
		}
	}
	return g
}

func (r *Result) platformBlockedCode() (code Code, ok bool) {
	switch {
	case r.target == Freestanding:
		return cTargetFreestanding, true
	case r.noPlatform:
		return cSysNonePlatform, true
	default:
		return Code{}, false
	}
}

// Reaching a Platform package transitively is as unavailable as importing
// it. std's own packages are exempt: `_bare.kg` /
// `_hosted.kg` selection already prunes those edges.
func (r *Result) platformAvailability(from PackageID, path string) (code Code, offendingPath string, ok bool) {
	if r.Packages[from].Std {
		return Code{}, "", false
	}
	start, exists := r.pathIndex[path]
	if !exists || !r.Packages[start].Std {
		return Code{}, "", false
	}
	blockCode, blocked := r.platformBlockedCode()
	if !blocked {
		return Code{}, "", false
	}
	seen := map[PackageID]bool{start: true}
	queue := []PackageID{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if stdLayer[r.Packages[cur].Path] == LayerPlatform {
			return blockCode, r.Packages[cur].Path, true
		}
		for _, next := range r.stdImportGraph[cur] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return Code{}, "", false
}
