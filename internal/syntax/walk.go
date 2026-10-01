package syntax

import "kigumi/internal/token"

// EachChild calls f for every child node of id, in slot order.
func (t *Tree) EachChild(id NodeID, f func(NodeID)) {
	n := t.Nodes[id]
	visit := func(c uint32) {
		if c != 0 {
			f(NodeID(c))
		}
	}
	switch shapes[n.Kind] {
	case sL:
		visit(n.Lhs)
	case sLR:
		visit(n.Lhs)
		visit(n.Rhs)
	case sFlagR, sTokR:
		visit(n.Rhs)
	case sList:
		for _, c := range t.Children(id) {
			f(c)
		}
	case sRec:
		for i, name := range recFields[n.Kind] {
			if name[0] != '#' && name[0] != '@' {
				visit(t.Extra[n.Lhs+uint32(i)])
			}
		}
	}
}

// SlotNames returns the record slot names of an sRec kind ("#x" flag slots,
// "@x" token slots, others child nodes), or nil.
func SlotNames(k NodeKind) []string { return recFields[k] }

// HasErrors reports whether parsing produced at least one error.
func (t *Tree) HasErrors() bool {
	for _, d := range t.Diags {
		if d.Severity == 0 {
			return true
		}
	}
	return false
}

// HasTopLevelStatements reports whether the file is an entry file candidate:
// it has a bare statement or `let` at top level.
func (t *Tree) HasTopLevelStatements() bool {
	for _, d := range t.Children(t.Root) {
		switch t.Kind(d) {
		case LetStmt, ExprStmt, AssignStmt, DeferStmt, ErrdeferStmt:
			return true
		}
	}
	return false
}

// HasExplicitMain reports whether the file declares the other entry form:
// a top-level, receiverless `fn main`.
func (t *Tree) HasExplicitMain() bool {
	for _, d := range t.Children(t.Root) {
		if t.Kind(d) != FnDecl || t.Slot(d, "recv") != 0 {
			continue
		}
		if name := t.Slot(d, "name"); name != 0 && t.TokText(name) == "main" {
			return true
		}
	}
	return false
}

// VisKind decodes a Visibility node: kind is "pub", "module", "super" or "in";
// path is the ancestor Path for "in".
func (t *Tree) VisKind(id NodeID) (kind string, path NodeID) {
	n := t.Nodes[id]
	if n.Kind != Visibility {
		return "", 0
	}
	if n.Lhs == 0 {
		return "pub", 0
	}
	return t.TokText(n.Lhs), NodeID(n.Rhs)
}

// StringPart is one piece of a string literal: literal text (still escaped,
// decode with DecodeString) or an interpolated expression.
type StringPart struct {
	Text token.Span
	Expr NodeID
}

// StringParts splits a StringLit into text spans and interpolation nodes.
func (t *Tree) StringParts(id NodeID) []StringPart {
	n := t.Nodes[id]
	tk := t.Toks[n.Tok]
	var exprs []NodeID
	if n.Lhs != 0 {
		exprs = t.Children(NodeID(n.Lhs))
	}
	var parts []StringPart
	pos := int(tk.Start) + 1
	for i, r := range interpolationRanges(t.File.Src, int(tk.Start), int(tk.End), t.interp) {
		if start := r[0] - 2; start > pos {
			parts = append(parts, StringPart{Text: token.Span{Start: token.Pos(pos), End: token.Pos(start)}})
		}
		if i < len(exprs) {
			parts = append(parts, StringPart{Expr: exprs[i]})
		}
		pos = r[1] + 1
	}
	if end := int(tk.End) - 1; end > pos {
		parts = append(parts, StringPart{Text: token.Span{Start: token.Pos(pos), End: token.Pos(end)}})
	}
	return parts
}
