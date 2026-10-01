package lsp

import (
	"sort"
	"strings"

	"go.lsp.dev/protocol"

	"kigumi/internal/driver"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// importIndex maps what a file could import: qualifiers to the paths they
// name, and public names to the paths declaring them.
type importIndex struct {
	qualifiers map[string][]string
	names      map[string][]namedDecl
}

type namedDecl struct {
	path string
	kind protocol.CompletionItemKind
}

func buildIndex(mod *driver.Module, current string) importIndex {
	ix := importIndex{qualifiers: map[string][]string{}, names: map[string][]namedDecl{}}
	paths := append([]string{}, mod.Order...)
	sort.SliceStable(paths, func(i, j int) bool { return sourceRank(mod.Packages[paths[i]]) < sourceRank(mod.Packages[paths[j]]) })
	for _, path := range paths {
		pkg := mod.Packages[path]
		if path == current || path == "std/prelude" || isEntryPackage(pkg) {
			continue
		}
		q := path[strings.LastIndex(path, "/")+1:]
		ix.qualifiers[q] = append(ix.qualifiers[q], path)
		for _, t := range pkg.Files {
			for _, d := range t.Children(t.Root) {
				name, kind, tok, ok := declSymbol(t, d)
				if !ok || strings.Contains(name, ".") || !exported(t, d, pkg.Module == "" && !pkg.Std) {
					continue
				}
				ix.names[name] = append(ix.names[name], namedDecl{path: path, kind: completionKind(kind)})
				_ = tok
			}
		}
	}
	return ix
}

func sourceRank(p *driver.Package) int {
	switch {
	case p.Std:
		return 0
	case p.Module == "":
		return 1
	}
	return 2
}

func isEntryPackage(p *driver.Package) bool {
	for _, t := range p.Files {
		if t.HasTopLevelStatements() || t.HasExplicitMain() {
			return true
		}
	}
	return false
}

// exported accepts `pub` declarations, and `pub(module)` ones when the
// importing file is in the same module.
func exported(t *syntax.Tree, d syntax.NodeID, sameModule bool) bool {
	vis := syntax.NodeID(slotTok(t, d, "vis"))
	if vis == 0 {
		return false
	}
	kind, _ := t.VisKind(vis)
	return kind == "pub" || (kind == "module" && sameModule)
}

func completionKind(k protocol.SymbolKind) protocol.CompletionItemKind {
	switch k {
	case protocol.SymbolKindFunction:
		return protocol.CompletionItemKindFunction
	case protocol.SymbolKindInterface:
		return protocol.CompletionItemKindInterface
	case protocol.SymbolKindConstant:
		return protocol.CompletionItemKindConstant
	}
	return protocol.CompletionItemKindClass
}

func importEdit(t *syntax.Tree, path, name string) protocol.TextEdit {
	var lastImport syntax.NodeID
	for _, d := range t.Children(t.Root) {
		if t.Kind(d) != syntax.ImportDecl {
			continue
		}
		lastImport = d
		s := t.Slots(d)
		if name != "" && importPathText(t, syntax.NodeID(s[2])) == path && t.Kind(syntax.NodeID(s[1])) == syntax.List {
			items := []importItem{{key: name, text: name}}
			for _, nm := range t.Children(syntax.NodeID(s[1])) {
				items = append(items, importItem{key: t.TokText(t.Nodes[nm].Tok), text: importItemText(t, nm)})
			}
			sort.Slice(items, func(i, j int) bool { return items[i].key < items[j].key })
			names := make([]string, len(items))
			for i, it := range items {
				names[i] = it.text
			}
			sp := t.Span(syntax.NodeID(s[1]))
			sp.End++ // the closing brace is not a node
			return protocol.TextEdit{Range: spanRange(t.File, sp), NewText: "{" + strings.Join(names, ", ") + "}"}
		}
	}
	line := "import " + path[strings.LastIndex(path, "/")+1:] + " from " + path + "\n"
	if name != "" {
		line = "import {" + name + "} from " + path + "\n"
	}
	var at token.Pos
	switch stmts := t.Children(t.Root); {
	case lastImport != 0:
		at = token.Pos(lineEndOf(t.File.Src, int(t.Span(lastImport).End)))
	case len(stmts) > 0:
		at = token.Pos(lineStartOf(t.File.Src, int(t.Span(stmts[0]).Start)))
		line += "\n"
	default:
		at = token.Pos(len(t.File.Src))
	}
	return protocol.TextEdit{Range: spanRange(t.File, token.Span{Start: at, End: at}), NewText: line}
}

// importItem is one `{}` binding item sorted by its local (bound) name;
// key and text differ only for a renamed import (`orig as alias`).
type importItem struct{ key, text string }

func importItemText(t *syntax.Tree, nm syntax.NodeID) string {
	n := t.Nodes[nm]
	if t.Kind(nm) != syntax.ImportAlias {
		return t.TokText(n.Tok)
	}
	return t.TokText(t.Nodes[n.Lhs].Tok) + " as " + t.TokText(n.Tok)
}

func importPathText(t *syntax.Tree, path syntax.NodeID) string {
	var segs []string
	for _, tk := range t.PathToks(path) {
		segs = append(segs, t.TokText(tk))
	}
	return strings.Join(segs, "/")
}

func lineEndOf(src []byte, pos int) int {
	for pos < len(src) && src[pos-1] != '\n' {
		pos++
	}
	return pos
}

func lineStartOf(src []byte, pos int) int {
	for pos > 0 && src[pos-1] != '\n' {
		pos--
	}
	return pos
}

// boundNames lists the qualifiers and selected names a file already
// imports, so completion does not offer to import them again.
func boundNames(t *syntax.Tree) map[string]bool {
	out := map[string]bool{}
	for _, d := range t.Children(t.Root) {
		if t.Kind(d) != syntax.ImportDecl {
			continue
		}
		b := syntax.NodeID(t.Slots(d)[1])
		if t.Kind(b) == syntax.List {
			for _, nm := range t.Children(b) {
				out[t.TokText(t.Nodes[nm].Tok)] = true
			}
			continue
		}
		out[t.TokText(t.Nodes[b].Tok)] = true
	}
	return out
}
