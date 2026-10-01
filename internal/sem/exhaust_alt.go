package sem

import "kigumi/internal/syntax"

// abstractAll expands `|` alternatives, including ones nested in
// constructor arguments or record fields, into the list of plain patterns
// an arm stands for.
func (c *checker) abstractAll(p syntax.NodeID) []pat {
	node := c.t.Nodes[p]
	if c.info.Types[p] == TyPoison {
		return []pat{{kind: pErr}}
	}
	switch node.Kind {
	case syntax.PatOr:
		var out []pat
		for _, alt := range c.t.Children(p) {
			out = append(out, c.abstractAll(alt)...)
		}
		return out
	case syntax.PatCtor:
		ent := c.info.Uses[p]
		if ent == 0 || c.r.Entities[ent].Kind != EntVariant || node.Rhs == 0 {
			return []pat{c.abstract(p)}
		}
		var columns [][]pat
		for _, s := range c.t.Children(syntax.NodeID(node.Rhs)) {
			columns = append(columns, c.abstractAll(s))
		}
		var out []pat
		for _, combo := range product(columns) {
			vi := c.r.variant(ent)
			args := wilds(len(vi.Payload))
			copy(args, combo)
			out = append(out, pat{kind: pCtor, key: variantKey(ent), name: c.r.Entities[ent].Name, args: args})
		}
		return out
	case syntax.PatRecord:
		ent := c.info.Uses[p]
		if ent == 0 {
			return []pat{{kind: pWild}}
		}
		var names []string
		var columns [][]pat
		for _, f := range c.t.Children(syntax.NodeID(node.Rhs)) {
			fn := c.t.Nodes[f]
			if fn.Lhs != 0 {
				names = append(names, c.t.TokText(fn.Tok))
				columns = append(columns, c.abstractAll(syntax.NodeID(fn.Lhs)))
			}
		}
		var out []pat
		for _, combo := range product(columns) {
			subs := map[string]pat{}
			for i, name := range names {
				subs[name] = combo[i]
			}
			out = append(out, c.recordPat(ent, subs))
		}
		return out
	case syntax.PatTuple:
		ent := c.info.Uses[p]
		if ent == 0 {
			return []pat{{kind: pWild}}
		}
		var columns [][]pat
		for _, s := range c.t.Children(p) {
			columns = append(columns, c.abstractAll(s))
		}
		var out []pat
		for _, combo := range product(columns) {
			out = append(out, pat{kind: pCtor, key: recordKey(ent), name: c.r.Entities[ent].Name, args: combo})
		}
		return out
	}
	return []pat{c.abstract(p)}
}

// product lists every choice of one pattern per column.
func product(columns [][]pat) [][]pat {
	out := [][]pat{nil}
	for _, col := range columns {
		var next [][]pat
		for _, prefix := range out {
			for _, alt := range col {
				row := append(append([]pat{}, prefix...), alt)
				next = append(next, row)
			}
		}
		out = next
	}
	return out
}
