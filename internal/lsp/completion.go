package lsp

import (
	"context"
	"sort"

	"go.lsp.dev/protocol"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// memberCompletion answers a request right after a dot with the receiver's
// members only; ok is false without a member position or a known receiver.
func (s *Server) memberCompletion(ctx context.Context, p protocol.TextDocumentPositionParams) (protocol.CompletionItemSlice, bool) {
	text, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return nil, false
	}
	f := fileOfText(text)
	pos := offset(f, p.Position)
	dot := dotBefore(f.Src, pos)
	if dot < 0 {
		return nil, false
	}
	snap := s.loadDoc(ctx, p.TextDocument.URI)
	if snap.mod == nil || snap.res == nil {
		return nil, false
	}
	t := snap.fileOf(uriToPath(p.TextDocument.URI))
	if t == nil {
		return nil, false
	}
	recv := receiverAt(t, token.Pos(dot))
	if recv == 0 {
		return nil, false
	}
	info := snap.res.File(t)
	static := false
	if ent := info.Uses[recv]; ent != 0 {
		switch snap.res.Entity(ent).Kind {
		case sem.EntType, sem.EntAlias:
			static = true
		}
	}
	rt := info.Types[recv]
	if static {
		rt = snap.res.Entity(info.Uses[recv]).Type
	}
	if rt == 0 {
		return nil, false
	}
	var items protocol.CompletionItemSlice
	for _, m := range snap.res.Members(rt, static) {
		e := snap.res.Entity(m.Ent)
		kind := protocol.CompletionItemKindMethod
		switch e.Kind {
		case sem.EntField:
			kind = protocol.CompletionItemKindField
		case sem.EntVariant:
			kind = protocol.CompletionItemKindEnumMember
		case sem.EntFn:
			if snap.res.Fn(m.Ent).Recv == sem.RecvNone {
				kind = protocol.CompletionItemKindFunction
			}
		}
		items = append(items, protocol.CompletionItem{Label: m.Name, Kind: kind, Detail: protocol.NewOptional(describe(snap.res, m.Ent, 0))})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return items, true
}

// dotBefore is the offset of the `.` the cursor's identifier hangs on:
// the cursor sits right after it, or after the letters typed since.
func dotBefore(src []byte, pos token.Pos) int {
	i := int(pos)
	for i > 0 && isIdentByte(src[i-1]) {
		i--
	}
	if i > 0 && src[i-1] == '.' {
		return i - 1
	}
	return -1
}

func isIdentByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= 0x80
}

// receiverAt: the innermost node ending right before the dot wins, so
// `a.b.` names `a.b`.
func receiverAt(t *syntax.Tree, pos token.Pos) syntax.NodeID {
	best, bestLen := syntax.NodeID(0), -1
	for id := 1; id < len(t.Nodes); id++ {
		n := syntax.NodeID(id)
		switch t.Kind(n) {
		case syntax.Ident, syntax.MemberExpr, syntax.CallExpr, syntax.BracketExpr, syntax.Paren, syntax.StringLit, syntax.ByteStringLit, syntax.IntLit, syntax.FloatLit:
		default:
			continue
		}
		sp := t.Span(n)
		if t.Kind(n) == syntax.MemberExpr {
			// A member node's span covers only its name token; the whole
			// expression ends where the name does.
			sp.Start = t.Span(syntax.NodeID(t.Nodes[n].Lhs)).Start
			sp.End = t.Toks[t.Nodes[n].Tok].Span.End
		}
		if sp.End != pos || sp.End <= sp.Start {
			continue
		}
		if bestLen < 0 || sp.Len() < bestLen {
			best, bestLen = n, sp.Len()
		}
	}
	return best
}

// scopeItems lists the names visible at the position, innermost scope
// first, each with the kind and type the checker knows.
func (s *Server) scopeItems(snap *snapshot, p protocol.TextDocumentPositionParams) []protocol.CompletionItem {
	t := snap.fileOf(uriToPath(p.TextDocument.URI))
	if t == nil {
		return nil
	}
	info := snap.res.File(t)
	pos := offset(t.File, p.Position)
	var scope sem.ScopeID
	bestLen := -1
	for id := 1; id < len(t.Nodes); id++ {
		n := syntax.NodeID(id)
		sc := info.Scopes[n]
		if sc == 0 {
			continue
		}
		sp := t.Span(n)
		if pos < sp.Start || pos > sp.End {
			continue
		}
		if bestLen < 0 || sp.Len() < bestLen {
			scope, bestLen = sc, sp.Len()
		}
	}
	if scope == 0 {
		scope = info.Scopes[t.Root]
	}
	var items []protocol.CompletionItem
	seen := map[string]bool{}
	for sc := scope; sc != 0; sc = snap.res.Scopes[sc].Parent {
		for name, b := range snap.res.Scopes[sc].Names {
			if seen[name] || name == "" || name[0] == '$' {
				continue
			}
			seen[name] = true
			item := protocol.CompletionItem{Label: name, Kind: protocol.CompletionItemKindVariable}
			ent := b.Ent
			if ent == 0 && b.Set != 0 && len(snap.res.Overloads[b.Set].Members) > 0 {
				ent = snap.res.Overloads[b.Set].Members[0]
			}
			if ent != 0 {
				item.Kind = completionKindOf(snap.res.Entity(ent).Kind)
				item.Detail = protocol.NewOptional(describe(snap.res, ent, 0))
			}
			items = append(items, item)
		}
	}
	return items
}

func completionKindOf(k sem.EntityKind) protocol.CompletionItemKind {
	switch k {
	case sem.EntFn:
		return protocol.CompletionItemKindFunction
	case sem.EntType, sem.EntAlias:
		return protocol.CompletionItemKindClass
	case sem.EntInterface:
		return protocol.CompletionItemKindInterface
	case sem.EntConst:
		return protocol.CompletionItemKindConstant
	case sem.EntVariant:
		return protocol.CompletionItemKindEnumMember
	case sem.EntPackage, sem.EntImport:
		return protocol.CompletionItemKindModule
	}
	return protocol.CompletionItemKindVariable
}
