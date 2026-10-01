package hir

import (
	"fmt"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// A spread lists the fields it fills, in declaration order, skipping the
// ones already set before it.
func (l *lowerer) record(n syntax.NodeID) *Expr {
	node := l.t.Nodes[n]
	call := l.info.Calls[n]
	info := l.r.TypeDecl(call.Callee)
	out := l.node(n, Record)
	out.Ent = call.Callee
	set := make([]bool, len(info.Fields))
	for _, entry := range l.t.Children(syntax.NodeID(node.Rhs)) {
		en := l.t.Nodes[entry]
		re := &RecordEntry{Node: entry, Value: l.expr(syntax.NodeID(en.Lhs))}
		if en.Kind != syntax.Spread {
			re.Index = l.r.Field(l.info.Uses[entry]).Index
			set[re.Index] = true
			out.Entries = append(out.Entries, re)
			continue
		}
		re.Spread = true
		srcType := l.typeOf(syntax.NodeID(en.Lhs))
		if l.r.Types.Node(srcType).Kind == sem.KRef {
			srcType = l.r.Types.Node(srcType).Elem
		}
		srcEnt := l.r.Types.Node(srcType).Ent
		for i, fld := range info.Fields {
			if set[i] {
				continue
			}
			sf := l.r.FindField(srcEnt, l.r.Entity(fld).Name)
			if sf == 0 {
				continue
			}
			re.Fields = append(re.Fields, SpreadField{Index: i, SrcIndex: l.r.Field(sf).Index, Type: l.r.Entity(fld).Type})
			set[i] = true
		}
		out.Entries = append(out.Entries, re)
	}
	return out
}

// tupleLit reuses Record's shape over the checker's predeclared TupleN
// entity; each element sets the field at its position.
func (l *lowerer) tupleLit(n syntax.NodeID) *Expr {
	call := l.info.Calls[n]
	out := l.node(n, Record)
	out.Ent = call.Callee
	for i, item := range l.t.Children(n) {
		out.Entries = append(out.Entries, &RecordEntry{Node: item, Index: i, Value: l.expr(item)})
	}
	return out
}

// shellLit records the plan words: each text piece as a string, each
// interpolation as an argument; "|" separates commands.
func (l *lowerer) shellLit(n syntax.NodeID) *Expr {
	node := l.t.Nodes[n]
	out := l.node(n, Shell)
	for i, cmd := range l.t.Children(syntax.NodeID(node.Rhs)) {
		if i > 0 {
			out.Strs = append(out.Strs, "|")
		}
		for _, part := range l.t.Children(cmd) {
			word := part
			if l.t.Kind(part) == syntax.ShellRedirect {
				out.Strs = append(out.Strs, fmt.Sprintf("r%d:%d", l.t.Slot(part, "op"), l.t.Slot(part, "fd")))
				word = syntax.NodeID(l.t.Slot(part, "target"))
			} else {
				out.Strs = append(out.Strs, " ")
			}
			for _, piece := range l.t.Children(word) {
				pn := l.t.Nodes[piece]
				if pn.Kind == syntax.ShellText {
					out.Strs = append(out.Strs, "="+syntax.DecodeString(string(l.t.File.Src[pn.Lhs:pn.Rhs])))
					continue
				}
				out.Strs = append(out.Strs, "$")
				out.Args = append(out.Args, l.expr(syntax.NodeID(pn.Lhs)))
			}
		}
	}
	return out
}
