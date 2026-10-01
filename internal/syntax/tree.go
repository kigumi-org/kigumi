package syntax

import (
	"kigumi/internal/diag"
	"kigumi/internal/token"
)

// NodeID indexes Tree.Nodes; 0 is the absent node.
type NodeID uint32

type Node struct {
	Kind NodeKind
	Tok  uint32
	Lhs  uint32
	Rhs  uint32
}

// Tree is the flat, lossless syntax tree. Nodes reference each other and the
// token array by index only; semantic information is kept in side tables
// owned by later passes.
type Tree struct {
	File  *token.File
	Toks  []token.Token
	Nodes []Node
	Extra []uint32
	Root  NodeID
	Diags []diag.Diagnostic
	// interp is the parser's interpolation-range cache (see parser.interp),
	// kept so StringParts reuses it instead of rescanning nested strings.
	interp map[int]interpInfo
	// spanCache memoizes Span per node id: every hir/sem/mir location lookup
	// calls Span, and without caching a chain of D nested nodes costs O(D)
	// per lookup, O(D²) total across the chain.
	spanCache []token.Span
	spanKnown []bool
}

func newTree(f *token.File, toks []token.Token) *Tree {
	return &Tree{File: f, Toks: toks, Nodes: make([]Node, 1, 64)}
}

func (t *Tree) add(n Node) NodeID {
	t.Nodes = append(t.Nodes, n)
	return NodeID(len(t.Nodes) - 1)
}

// AddList appends a list node; the checker uses it to group nodes.
func (t *Tree) AddList(kind NodeKind, tok uint32, items []NodeID) NodeID {
	return t.addList(kind, tok, items)
}

func (t *Tree) addList(kind NodeKind, tok uint32, items []NodeID) NodeID {
	start := uint32(len(t.Extra))
	for _, it := range items {
		t.Extra = append(t.Extra, uint32(it))
	}
	return t.add(Node{Kind: kind, Tok: tok, Lhs: start, Rhs: uint32(len(t.Extra))})
}

func (t *Tree) addRec(kind NodeKind, tok uint32, slots []uint32) NodeID {
	start := uint32(len(t.Extra))
	t.Extra = append(t.Extra, slots...)
	return t.add(Node{Kind: kind, Tok: tok, Lhs: start, Rhs: uint32(len(t.Extra))})
}

func (p *parser) leaf(kind NodeKind, tok uint32) NodeID {
	return p.tree.add(Node{Kind: kind, Tok: tok})
}

func (p *parser) node1(kind NodeKind, tok uint32, lhs NodeID) NodeID {
	return p.tree.add(Node{Kind: kind, Tok: tok, Lhs: uint32(lhs)})
}

func (p *parser) node2(kind NodeKind, tok uint32, lhs, rhs NodeID) NodeID {
	return p.tree.add(Node{Kind: kind, Tok: tok, Lhs: uint32(lhs), Rhs: uint32(rhs)})
}

func (p *parser) nodeFlag(kind NodeKind, tok uint32, flag uint32, rhs NodeID) NodeID {
	return p.tree.add(Node{Kind: kind, Tok: tok, Lhs: flag, Rhs: uint32(rhs)})
}

func (t *Tree) Node(id NodeID) Node { return t.Nodes[id] }

func (t *Tree) Kind(id NodeID) NodeKind {
	if id == 0 {
		return NoneKind
	}
	return t.Nodes[id].Kind
}

// Children returns the child node ids of a list node.
func (t *Tree) Children(id NodeID) []NodeID {
	n := t.Nodes[id]
	if shapes[n.Kind] != sList {
		return nil
	}
	out := make([]NodeID, 0, n.Rhs-n.Lhs)
	for _, e := range t.Extra[n.Lhs:n.Rhs] {
		out = append(out, NodeID(e))
	}
	return out
}

// Slots returns the raw record slots of an sRec node.
func (t *Tree) Slots(id NodeID) []uint32 {
	n := t.Nodes[id]
	if shapes[n.Kind] != sRec {
		return nil
	}
	return t.Extra[n.Lhs:n.Rhs]
}

// Slot returns the named slot of an sRec node, or 0.
func (t *Tree) Slot(id NodeID, name string) uint32 {
	n := t.Nodes[id]
	for i, f := range recFields[n.Kind] {
		if f[1:] == name || f == name {
			return t.Extra[n.Lhs+uint32(i)]
		}
	}
	return 0
}

// PathToks returns the token indices of a Path node.
func (t *Tree) PathToks(id NodeID) []uint32 {
	n := t.Nodes[id]
	if n.Kind != Path {
		return nil
	}
	return t.Extra[n.Lhs:n.Rhs]
}

func (t *Tree) TokText(idx uint32) string {
	return t.Toks[idx].Text(t.File.Src)
}
