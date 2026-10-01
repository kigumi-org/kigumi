package sem

import "kigumi/internal/syntax"

// checkNomono (GEN-7) rejects polymorphic recursion, after Featherweight Go's nomono check.
func (r *Result) checkNomono() {
	g := newInstGraph(r)
	g.build()
	g.report()
}

type instVertex struct {
	fn    EntityID
	param int
}

type instEdge struct {
	from, to instVertex
	grow     bool
	file     FileID
	node     uint32
	arg      TypeID
}

type instGraph struct {
	r     *Result
	index map[instVertex]int
	verts []instVertex
	edges []instEdge
	adj   map[int][]int
}

func newInstGraph(r *Result) *instGraph {
	return &instGraph{r: r, index: map[instVertex]int{}, adj: map[int][]int{}}
}

func (g *instGraph) vertex(v instVertex) int {
	if i, ok := g.index[v]; ok {
		return i
	}
	g.index[v] = len(g.verts)
	g.verts = append(g.verts, v)
	return len(g.verts) - 1
}

func (g *instGraph) build() {
	r := g.r
	tt := r.Types
	for callee, insts := range r.Instances {
		for _, in := range insts {
			if in.In == 0 || r.Entities[in.In].Kind != EntFn {
				continue
			}
			caller := r.Fn(in.In)
			for j, arg := range in.Args {
				for i, p := range caller.TypeParams {
					pt := tt.Param(p)
					if !tt.Contains(arg, pt) {
						continue
					}
					from := g.vertex(instVertex{in.In, i})
					to := g.vertex(instVertex{callee, j})
					g.edges = append(g.edges, instEdge{from: g.verts[from], to: g.verts[to], grow: arg != pt, file: in.File, node: uint32(in.Node), arg: arg})
					g.adj[from] = append(g.adj[from], to)
				}
			}
		}
	}
}

func (g *instGraph) report() {
	comp := g.scc()
	seen := map[instVertex]bool{}
	for _, e := range g.edges {
		if !e.grow || comp[g.index[e.from]] != comp[g.index[e.to]] || seen[e.from] {
			continue
		}
		seen[e.from] = true
		r := g.r
		f := r.Entities[e.from.fn].Name + "[" + r.Entities[r.Fn(e.from.fn).TypeParams[e.from.param]].Name + "]"
		gname := r.Entities[e.to.fn].Name + "[" + r.TypeString(e.arg) + "]"
		r.errAt(e.file, syntax.NodeID(e.node), cNomonoRecursion, f, gname)
	}
}

// scc is Tarjan's algorithm over the adjacency lists.
func (g *instGraph) scc() []int {
	n := len(g.verts)
	index := make([]int, n)
	low := make([]int, n)
	comp := make([]int, n)
	onStack := make([]bool, n)
	for i := range index {
		index[i] = -1
	}
	var stack []int
	next, comps := 0, 0
	var visit func(v int)
	visit = func(v int) {
		index[v], low[v] = next, next
		next++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range g.adj[v] {
			if index[w] < 0 {
				visit(w)
				low[v] = min(low[v], low[w])
			} else if onStack[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] == index[v] {
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp[w] = comps
				if w == v {
					break
				}
			}
			comps++
		}
	}
	for v := range g.verts {
		if index[v] < 0 {
			visit(v)
		}
	}
	return comp
}
