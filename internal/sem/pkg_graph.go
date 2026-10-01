package sem

import (
	"slices"
	"strings"

	"kigumi/internal/syntax"
)

// buildPackageGraph computes the strongly connected components of the
// import graph (Tarjan). SCCs come out dependencies-first, which is the
// order every later pass uses; a component spanning two modules is a
// module cycle.
func (r *Result) buildPackageGraph() {
	n := len(r.Packages)
	index := make([]int, n)
	low := make([]int, n)
	onStack := make([]bool, n)
	var stack []PackageID
	counter := 1
	r.order = r.order[:0]
	sccCount := 0
	var strong func(v PackageID)
	strong = func(v PackageID) {
		index[v], low[v] = counter, counter
		counter++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range r.importEdges[v] {
			if index[w] == 0 {
				strong(w)
				low[v] = min(low[v], low[w])
			} else if onStack[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] != index[v] {
			return
		}
		var comp []PackageID
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[w] = false
			comp = append(comp, w)
			if w == v {
				break
			}
		}
		sccCount++
		slices.Sort(comp)
		for _, w := range comp {
			r.Packages[w].SCC = sccCount
			r.order = append(r.order, w)
		}
		r.checkModuleCycle(comp)
	}
	for v := 1; v < n; v++ {
		if index[v] == 0 {
			strong(PackageID(v))
		}
	}
}

func (r *Result) checkModuleCycle(comp []PackageID) {
	if len(comp) < 2 {
		return
	}
	first := r.Packages[comp[0]].Module
	crossing := false
	for _, p := range comp[1:] {
		if r.Packages[p].Module != first {
			crossing = true
		}
	}
	if !crossing {
		return
	}
	var names []string
	for _, p := range comp {
		names = append(names, r.Packages[p].Path)
	}
	chain := strings.Join(names, " -> ")
	inComp := map[PackageID]bool{}
	for _, p := range comp {
		inComp[p] = true
	}
	for _, p := range comp {
		for _, f := range r.Packages[p].Files {
			t := r.tree(f)
			for _, d := range r.topDecls(f) {
				if t.Kind(d) != syntax.ImportDecl {
					continue
				}
				target, ok := r.pathIndex[pathText(t, importDecl(t, d).Path, "/")]
				if ok && inComp[target] && !r.sameModule(p, target) {
					r.errAt(f, d, cModuleCycle, chain)
				}
			}
		}
	}
}
