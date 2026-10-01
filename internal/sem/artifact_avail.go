package sem

// LayerShortName maps a layer's short name to
// its Layer value; "" (no ceiling) is deliberately not a key here.
var LayerShortName = map[string]Layer{
	"core":  LayerFoundation,
	"alloc": LayerAllocation,
	"sys":   LayerPlatform,
	"tool":  LayerTool,
}

func denySet(paths []string) map[string]bool {
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[p] = true
	}
	return set
}

func (r *Result) artifactBlocked(path string) bool {
	if r.denyPkgs[path] {
		return true
	}
	return r.hasCeiling && stdLayer[path] > r.layerCeiling
}

// artifactAvailability reports the diagnostic to raise for an import of
// path from package from, or ok=false when nothing declared blocks it. It
// walks the std import graph like platformAvailability; std's own imports
// are exempt because platform-file selection already pruned what a build
// cannot reach.
func (r *Result) artifactAvailability(from PackageID, path string) (offendingPath string, ok bool) {
	if r.Packages[from].Std || (len(r.denyPkgs) == 0 && !r.hasCeiling) {
		return "", false
	}
	start, exists := r.pathIndex[path]
	if !exists || !r.Packages[start].Std {
		return "", false
	}
	seen := map[PackageID]bool{start: true}
	queue := []PackageID{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if r.artifactBlocked(r.Packages[cur].Path) {
			return r.Packages[cur].Path, true
		}
		for _, next := range r.stdImportGraph[cur] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return "", false
}
