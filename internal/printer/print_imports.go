package printer

import (
	"sort"
	"strings"

	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// sortImports orders the file's first run of import declarations: std,
// then host-qualified paths, then `pub import`, each by path.
// The run stays in source order when a comment sits inside it, since the
// comment could not follow its declaration. tight marks the imports that
// print without a blank line before them.
func (p *printer) sortImports(decls []syntax.NodeID) ([]syntax.NodeID, map[syntax.NodeID]bool) {
	tight := map[syntax.NodeID]bool{}
	start := -1
	end := 0
	for i, d := range decls {
		if p.t.Kind(d) == syntax.ImportDecl {
			if start < 0 {
				start = i
			}
			end = i + 1
		} else if start >= 0 {
			break
		}
	}
	if start < 0 || end-start < 2 || p.commentWithin(decls[start], decls[end-1]) {
		return decls, tight
	}
	run := append([]syntax.NodeID{}, decls[start:end]...)
	sort.SliceStable(run, func(i, j int) bool {
		gi, gj := p.importGroup(run[i]), p.importGroup(run[j])
		if gi != gj {
			return gi < gj
		}
		return p.importPath(run[i]) < p.importPath(run[j])
	})
	out := append(append(append([]syntax.NodeID{}, decls[:start]...), run...), decls[end:]...)
	for _, d := range run[1:] {
		tight[d] = true
	}
	return out, tight
}

func (p *printer) importGroup(d syntax.NodeID) int {
	if p.t.Slots(d)[0] != 0 {
		return 2
	}
	if strings.HasPrefix(p.importPath(d), "std/") || p.importPath(d) == "std" {
		return 0
	}
	return 1
}

func (p *printer) importPath(d syntax.NodeID) string {
	var segs []string
	for _, tk := range p.t.PathToks(syntax.NodeID(p.t.Slots(d)[2])) {
		segs = append(segs, p.t.TokText(tk))
	}
	return strings.Join(segs, "/")
}

// commentWithin reports a comment token between the first token of a and
// the end of b.
func (p *printer) commentWithin(a, b syntax.NodeID) bool {
	from, to := p.t.Span(a).Start, p.t.Span(b).End
	for _, tk := range p.t.Toks {
		if tk.Kind == token.EOF {
			break
		}
		if (tk.Kind == token.Comment || tk.Kind == token.DocComment) && tk.Start >= from && tk.Start < to {
			return true
		}
	}
	return false
}

// sortedNames orders the `{a, b}` binding list of an import.
func (p *printer) sortedNames(names []syntax.NodeID) []syntax.NodeID {
	out := append([]syntax.NodeID{}, names...)
	sort.SliceStable(out, func(i, j int) bool { return p.tok(p.node(out[i]).Tok) < p.tok(p.node(out[j]).Tok) })
	return out
}
