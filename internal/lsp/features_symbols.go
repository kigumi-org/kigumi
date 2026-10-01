package lsp

import (
	"context"
	"sort"
	"strings"

	"go.lsp.dev/protocol"

	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func (s *Server) DocumentSymbol(ctx context.Context, p *protocol.DocumentSymbolParams) (protocol.DocumentSymbolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	text, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return nil, nil
	}
	t := syntax.Parse(token.NewFile(uriToPath(p.TextDocument.URI), []byte(text)))
	out := protocol.DocumentSymbolSlice{}
	for _, d := range t.Children(t.Root) {
		name, kind, tok, ok := declSymbol(t, d)
		if !ok {
			continue
		}
		sym := protocol.DocumentSymbol{Name: name, Kind: kind, Range: declRange(t, d, tok), SelectionRange: spanRange(t.File, t.Toks[tok].Span)}
		for _, m := range memberSymbols(t, d) {
			sym.Children = append(sym.Children, protocol.DocumentSymbol{Name: m.name, Kind: m.kind, Range: declRange(t, m.node, m.tok), SelectionRange: spanRange(t.File, t.Toks[m.tok].Span)})
		}
		out = append(out, sym)
	}
	return out, nil
}

// slotTok reads a slot of a record node by name (`@name`, `recv`, ...).
func slotTok(t *syntax.Tree, n syntax.NodeID, name string) uint32 {
	names := syntax.SlotNames(t.Kind(n))
	for i, s := range t.Slots(n) {
		if i < len(names) && strings.TrimLeft(names[i], "@#") == name {
			return s
		}
	}
	return 0
}

var keywords = []string{"fn", "let", "mut", "pub", "type", "interface", "const", "import", "from", "match", "if", "else", "for", "in",
	"return", "fail", "defer", "errdefer", "break", "continue", "is", "pure", "noalloc", "unsafe", "async", "await", "comptime",
	"allocator", "asm", "extern", "export", "test", "resource", "move", "self", "true", "false", "None", "Some", "Ok", "Err"}

// Completion offers keywords, names in scope, and then import candidates
// from the rest of the module; a name already available bare always wins
// over a same-named import suggestion.
func (s *Server) Completion(ctx context.Context, p *protocol.CompletionParams) (protocol.CompletionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if items, ok := s.memberCompletion(ctx, p.TextDocumentPositionParams); ok {
		return items, nil
	}
	seen := map[string]bool{}
	items := protocol.CompletionItemSlice{}
	add := func(label string, kind protocol.CompletionItemKind, detail string) {
		if seen[label] {
			return
		}
		seen[label] = true
		item := protocol.CompletionItem{Label: label, Kind: kind}
		if detail != "" {
			item.Detail = protocol.NewOptional(detail)
		}
		items = append(items, item)
	}
	for _, k := range keywords {
		add(k, protocol.CompletionItemKindKeyword, "")
	}
	snap := s.loadDoc(ctx, p.TextDocument.URI)
	// Scope items are already deduped by the checker; a file it could not
	// see falls back to raw package declarations, then plain identifiers.
	if snap.res != nil {
		for _, sc := range s.scopeItems(snap, p.TextDocumentPositionParams) {
			if !seen[sc.Label] {
				seen[sc.Label] = true
				items = append(items, sc)
			}
		}
	} else {
		if snap.mod != nil {
			path := snap.packageOf(uriToPath(p.TextDocument.URI))
			if pkg := snap.mod.Packages[path]; pkg != nil {
				for _, t := range pkg.Files {
					for _, d := range t.Children(t.Root) {
						name, kind, _, ok := declSymbol(t, d)
						if !ok || strings.Contains(name, ".") {
							continue
						}
						add(name, completionKind(kind), path)
					}
				}
			}
		}
		if text, ok := s.docs[p.TextDocument.URI]; ok {
			t := syntax.Parse(token.NewFile("", []byte(text)))
			for _, tok := range t.Toks {
				if tok.Kind == token.Ident {
					add(tok.Text(t.File.Src), protocol.CompletionItemKindVariable, "")
				}
			}
		}
	}
	// Auto-import candidates run last, so a local declaration or scope
	// binding is never shadowed by a same-named import suggestion.
	if snap.mod != nil {
		for _, item := range s.autoImportItems(snap, p.TextDocument.URI) {
			if !seen[item.Label] {
				seen[item.Label] = true
				items = append(items, item)
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return items, nil
}

// declRange covers the declaration and its name token; a node span omits
// token slots, so a bodiless `pub type Arena` would end before its name.
func declRange(t *syntax.Tree, d syntax.NodeID, nameTok uint32) protocol.Range {
	sp, name := t.Span(d), t.Toks[nameTok].Span
	sp.Start, sp.End = min(sp.Start, name.Start), max(sp.End, name.End)
	return spanRange(t.File, sp)
}
