package lsp

import (
	"path/filepath"
	"regexp"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"kigumi/internal/diag"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// autoImportItems are completion candidates from other packages; picking
// one also inserts its import.
func (s *Server) autoImportItems(snap *snapshot, u uri.URI) []protocol.CompletionItem {
	path := uriToPath(u)
	t := syntax.Parse(token.NewFile(path, []byte(s.docs[u])))
	ix := buildIndex(snap.mod, snap.packageOf(path))
	bound := boundNames(t)
	var items []protocol.CompletionItem
	for q, paths := range ix.qualifiers {
		if bound[q] {
			continue
		}
		items = append(items, protocol.CompletionItem{
			Label: q, Kind: protocol.CompletionItemKindModule, Detail: protocol.NewOptional("import " + q + " from " + paths[0]),
			AdditionalTextEdits: []protocol.TextEdit{importEdit(t, paths[0], "")},
		})
	}
	for name, decls := range ix.names {
		if bound[name] {
			continue
		}
		items = append(items, protocol.CompletionItem{
			Label: name, Kind: decls[0].kind, Detail: protocol.NewOptional("import {" + name + "} from " + decls[0].path),
			AdditionalTextEdits: []protocol.TextEdit{importEdit(t, decls[0].path, name)},
		})
	}
	return items
}

var backticked = regexp.MustCompile("`([^`]+)`")

// autoImportFixes turns an undefined name into "import ..." quick fixes:
// the packages it could qualify, then the packages declaring it.
func autoImportFixes(snap *snapshot, t *syntax.Tree, d diag.Diagnostic) []diag.Fix {
	if d.Code != "E100" {
		return nil
	}
	m := backticked.FindStringSubmatch(d.Msg)
	if m == nil {
		return nil
	}
	ix := buildIndex(snap.mod, snap.packageOf(filepath.Join(snap.root, filepath.FromSlash(t.File.Name))))
	var fixes []diag.Fix
	for _, path := range ix.qualifiers[m[1]] {
		e := importEdit(t, path, "")
		fixes = append(fixes, diag.Fix{Title: "import " + m[1] + " from " + path, Loc: diag.At(t.File, editSpan(t.File, e)), NewText: e.NewText})
	}
	for _, decl := range ix.names[m[1]] {
		e := importEdit(t, decl.path, m[1])
		fixes = append(fixes, diag.Fix{Title: "import {" + m[1] + "} from " + decl.path, Loc: diag.At(t.File, editSpan(t.File, e)), NewText: e.NewText})
	}
	return fixes
}

func editSpan(f *token.File, e protocol.TextEdit) token.Span {
	return token.Span{Start: offset(f, e.Range.Start), End: offset(f, e.Range.End)}
}

func (snap *snapshot) packageOf(path string) string {
	rel, err := filepath.Rel(snap.root, path)
	if err != nil {
		return ""
	}
	for p, pkg := range snap.mod.Packages {
		for _, t := range pkg.Files {
			if t.File.Name == filepath.ToSlash(rel) {
				return p
			}
		}
	}
	return ""
}
