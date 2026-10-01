package driver

import "kigumi/internal/syntax"

func importPaths(t *syntax.Tree) []string {
	var out []string
	for _, d := range t.Children(t.Root) {
		if t.Kind(d) != syntax.ImportDecl {
			continue
		}
		path := ""
		for i, tk := range t.PathToks(syntax.NodeID(t.Slots(d)[2])) {
			if i > 0 {
				path += "/"
			}
			path += t.TokText(tk)
		}
		out = append(out, path)
	}
	return out
}
