package sem

import (
	"slices"
	"sort"

	"kigumi/internal/diag"
	"kigumi/internal/syntax"
)

// Candidate must be within an edit distance of a third of name's length
// (one for short names); ties go to the alphabetically first candidate.
func suggest(name string, candidates []string) string {
	limit := max(1, len(name)/3)
	best, bestDist := "", limit+1
	sort.Strings(candidates)
	for _, c := range candidates {
		if c == name || c == "" || c[0] == '$' {
			continue
		}
		if d := editDistance(name, c); d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func (c *checker) visibleNames() []string {
	var out []string
	for s := c.scope; s != 0; s = c.r.Scopes[s].Parent {
		for name := range c.r.Scopes[s].Names {
			out = append(out, name)
		}
	}
	return out
}

func (r *Result) memberNames(ent EntityID) []string {
	if ent == 0 || r.Entities[ent].Kind != EntType {
		return nil
	}
	info := r.typeDecl(ent)
	var out []string
	for name := range info.Members {
		out = append(out, name)
	}
	for _, f := range info.Fields {
		out = append(out, r.Entities[f].Name)
	}
	for _, v := range info.Variants {
		out = append(out, r.Entities[v].Name)
	}
	return slices.Compact(out)
}

func (c *checker) errSuggest(n syntax.NodeID, code Code, name string, candidates []string, args ...any) {
	d, ok := c.r.diagAt(c.f, n, code, args...)
	if !ok {
		return
	}
	if alt := suggest(name, candidates); alt != "" {
		d = d.WithHelp("did you mean `" + alt + "`?")
		d.Fixes = append(d.Fixes, c.r.replaceFix(c.f, n, "replace with `"+alt+"`", alt))
	}
	c.r.emitDiag(c.f, d)
}

func (r *Result) replaceFix(f FileID, n syntax.NodeID, title, text string) diag.Fix {
	return diag.Fix{Title: title, Loc: r.loc(f, r.Files[f].span(n)), NewText: text}
}

func (r *Result) namedEnt(t TypeID) EntityID {
	if n := r.Types.Node(t); n.Kind == KNamed {
		return n.Ent
	}
	return 0
}
