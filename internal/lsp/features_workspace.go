package lsp

import (
	"context"
	"sort"
	"strings"
	"unicode"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"kigumi/internal/driver"
	"kigumi/internal/syntax"
)

const maxWorkspaceSymbols = 100

// declSymbol classifies a top-level declaration; methods are named
// `Type.method`. ok is false for declarations without a name.
func declSymbol(t *syntax.Tree, d syntax.NodeID) (name string, kind protocol.SymbolKind, nameTok uint32, ok bool) {
	switch t.Kind(d) {
	case syntax.FnDecl:
		kind = protocol.SymbolKindFunction
	case syntax.TypeDecl:
		kind = protocol.SymbolKindStruct
	case syntax.InterfaceDecl:
		kind = protocol.SymbolKindInterface
	case syntax.ConstDecl:
		kind = protocol.SymbolKindConstant
	default:
		return "", 0, 0, false
	}
	nameTok = slotTok(t, d, "name")
	if nameTok == 0 {
		return "", 0, 0, false
	}
	name = t.TokText(nameTok)
	if recv := slotTok(t, d, "recv"); recv != 0 && t.Kind(d) == syntax.FnDecl {
		sp := t.Span(syntax.NodeID(recv))
		name = string(t.File.Src[sp.Start:sp.End]) + "." + name
		kind = protocol.SymbolKindMethod
	}
	return name, kind, nameTok, true
}

type memberSymbol struct {
	name string
	kind protocol.SymbolKind
	node syntax.NodeID
	tok  uint32
}

// memberSymbols lists the requirements of an interface and the fields of a
// record type, which sit one list below the declaration.
func memberSymbols(t *syntax.Tree, d syntax.NodeID) []memberSymbol {
	var out []memberSymbol
	for _, slot := range []string{"members", "body"} {
		for _, m := range t.Children(syntax.NodeID(t.Slot(d, slot))) {
			switch t.Kind(m) {
			case syntax.FnDecl:
				if tok := slotTok(t, m, "name"); tok != 0 {
					out = append(out, memberSymbol{t.TokText(tok), protocol.SymbolKindMethod, m, tok})
				}
			case syntax.Field:
				if tok := t.Nodes[m].Tok; tok != 0 {
					out = append(out, memberSymbol{t.TokText(tok), protocol.SymbolKindField, m, tok})
				}
			}
		}
	}
	return out
}

// Symbols searches the top-level declarations of every module in the
// workspace with a fuzzy, smart-case match; like gopls it answers an
// empty query with nothing and caps the result at 100.
func (s *Server) Symbols(ctx context.Context, p *protocol.WorkspaceSymbolParams) (protocol.WorkspaceSymbolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := protocol.SymbolInformationSlice{}
	if p.Query == "" || s.root == "" {
		return out, nil
	}
	for _, root := range driver.ModuleRoots(s.root, s.stdRoot) {
		snap := s.load(ctx, root, "")
		if snap.mod == nil {
			continue
		}
		for _, path := range snap.mod.Order {
			pkg := snap.mod.Packages[path]
			if pkg.Std {
				continue
			}
			for _, t := range pkg.Files {
				add := func(name string, kind protocol.SymbolKind, tok uint32) {
					if !fuzzyMatch(p.Query, name) {
						return
					}
					container := path
					loc := protocol.Location{URI: uri.File(s.pathOfFile(snap, t.File.Name)), Range: spanRange(t.File, t.Toks[tok].Span)}
					out = append(out, protocol.SymbolInformation{BaseSymbolInformation: protocol.BaseSymbolInformation{Name: name, Kind: kind, ContainerName: &container}, Location: loc})
				}
				for _, d := range t.Children(t.Root) {
					name, kind, tok, ok := declSymbol(t, d)
					if !ok {
						continue
					}
					add(name, kind, tok)
					for _, m := range memberSymbols(t, d) {
						add(name+"."+m.name, m.kind, m.tok)
					}
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Name) != len(out[j].Name) {
			return len(out[i].Name) < len(out[j].Name)
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > maxWorkspaceSymbols {
		out = out[:maxWorkspaceSymbols]
	}
	return out, nil
}

// fuzzyMatch accepts name when query's characters appear in it in order;
// a query without upper-case letters matches case-insensitively.
func fuzzyMatch(query, name string) bool {
	if !strings.ContainsFunc(query, unicode.IsUpper) {
		query, name = strings.ToLower(query), strings.ToLower(name)
	}
	rest := name
	for _, q := range query {
		i := strings.IndexRune(rest, q)
		if i < 0 {
			return false
		}
		rest = rest[i+len(string(q)):]
	}
	return true
}
