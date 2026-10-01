package syntax

import "kigumi/internal/token"

// spanComputeOps counts Span cache misses; interp_perf_test.go asserts each
// node's span is computed once, not once per query.
var spanComputeOps int

func isShellInner(k NodeKind) bool {
	return k >= ShellPlan && k <= ShellRedirect
}

// Span returns the source range covered by a node and all its descendants,
// caching per id (see Tree.spanCache).
func (t *Tree) Span(id NodeID) token.Span {
	if id == 0 {
		return token.Span{}
	}
	if int(id) < len(t.spanKnown) && t.spanKnown[id] {
		return t.spanCache[id]
	}
	sp := t.computeSpan(id)
	if int(id) >= len(t.spanKnown) {
		grown := make([]bool, len(t.Nodes))
		copy(grown, t.spanKnown)
		t.spanKnown = grown
		cache := make([]token.Span, len(t.Nodes))
		copy(cache, t.spanCache)
		t.spanCache = cache
	}
	t.spanCache[id], t.spanKnown[id] = sp, true
	return sp
}

func (t *Tree) computeSpan(id NodeID) token.Span {
	spanComputeOps++
	n := t.Nodes[id]
	var sp token.Span
	if n.Kind == ShellText {
		return token.Span{Start: token.Pos(n.Lhs), End: token.Pos(n.Rhs)}
	}
	if n.Kind != Path && !isShellInner(n.Kind) {
		sp = t.Toks[n.Tok].Span
	}
	widen := func(c NodeID) {
		if c == 0 {
			return
		}
		cs := t.Span(c)
		if sp.End == 0 && sp.Start == 0 {
			sp = cs
			return
		}
		sp.Start, sp.End = min(sp.Start, cs.Start), max(sp.End, cs.End)
	}
	switch shapes[n.Kind] {
	case sL:
		widen(NodeID(n.Lhs))
	case sLR:
		widen(NodeID(n.Lhs))
		widen(NodeID(n.Rhs))
	case sFlagR, sTokR:
		widen(NodeID(n.Rhs))
	case sList:
		for _, c := range t.Children(id) {
			widen(c)
		}
	case sRec:
		for i, name := range recFields[n.Kind] {
			if name[0] != '#' && name[0] != '@' {
				widen(NodeID(t.Extra[n.Lhs+uint32(i)]))
			}
		}
	case sPath:
		for _, tk := range t.PathToks(id) {
			ts := t.Toks[tk].Span
			if sp == (token.Span{}) {
				sp = ts
			}
			sp.Start, sp.End = min(sp.Start, ts.Start), max(sp.End, ts.End)
		}
	}
	return sp
}
