package sem

import (
	"kigumi/internal/syntax"
)

// ctorSet lists the constructors of a column type; complete is false for
// unbounded domains (numbers, strings, existentials, opaque types).
func (c *checker) ctorSet(t TypeID) ([]ctorDesc, bool) {
	tt := c.r.Types
	t = c.vars.resolve(t)
	if t == TyBool {
		return []ctorDesc{{key: "true", name: "true"}, {key: "false", name: "false"}}, true
	}
	n := tt.Node(t)
	if n.Kind != KNamed {
		return nil, false
	}
	info := c.r.typeDecl(n.Ent)
	subst := map[EntityID]TypeID{}
	for i, p := range info.Params {
		if i < len(n.Args) {
			subst[p] = n.Args[i]
		}
	}
	switch info.Form {
	case FormAdt:
		var out []ctorDesc
		for _, v := range info.Variants {
			var args []TypeID
			for _, p := range c.r.variant(v).Payload {
				args = append(args, tt.Subst(p, subst))
			}
			out = append(out, ctorDesc{key: variantKey(v), name: c.r.Entities[v].Name, args: args})
		}
		return out, true
	case FormRecord, FormResource:
		var args []TypeID
		for _, f := range info.Fields {
			args = append(args, tt.Subst(c.r.Entities[f].Type, subst))
		}
		return []ctorDesc{{key: recordKey(n.Ent), name: c.r.Entities[n.Ent].Name, args: args}}, true
	}
	return nil, false
}

func variantKey(v EntityID) string { return "v" + itoa(int(v)) }
func recordKey(e EntityID) string  { return "r" + itoa(int(e)) }

// argTypes gives the column types introduced by specializing on h.
func (c *checker) argTypes(t TypeID, h pat) []TypeID {
	set, _ := c.ctorSet(t)
	for _, d := range set {
		if d.key == h.key {
			return append([]TypeID{}, d.args...)
		}
	}
	return wildTypes(len(h.args))
}

func wildTypes(n int) []TypeID {
	out := make([]TypeID, n)
	for i := range out {
		out[i] = TyPoison
	}
	return out
}

// abstract turns a checked pattern node into a matrix pattern.
func (c *checker) abstract(p syntax.NodeID) pat {
	node := c.t.Nodes[p]
	if c.info.Types[p] == TyPoison {
		return pat{kind: pErr}
	}
	switch node.Kind {
	case syntax.PatLit:
		return c.abstractLit(syntax.NodeID(node.Lhs))
	case syntax.PatCtor:
		ent := c.info.Uses[p]
		if ent == 0 || c.r.Entities[ent].Kind != EntVariant {
			if ent != 0 {
				return pat{kind: pLit, key: "t" + itoa(int(ent)), name: c.r.Entities[ent].Name}
			}
			return pat{kind: pWild}
		}
		vi := c.r.variant(ent)
		args := wilds(len(vi.Payload))
		for i, s := range c.t.Children(syntax.NodeID(node.Rhs)) {
			if i < len(args) {
				args[i] = c.abstract(s)
			}
		}
		return pat{kind: pCtor, key: variantKey(ent), name: c.r.Entities[ent].Name, args: args}
	case syntax.PatRecord:
		return c.abstractRecord(p)
	case syntax.PatTuple:
		return c.abstractTuple(p)
	case syntax.PatRange:
		// Each range gets its own key: ranges never make an integer match
		// exhaustive (defaultMatrix already excludes non-pWild rows), and
		// overlap between ranges, or a range and a literal, is not an error
		// so a range's key never collides with another row's.
		return pat{kind: pRange, key: "g" + itoa(int(p))}
	}
	return pat{kind: pWild}
}

// abstractTuple orders the field patterns by position: a tuple pattern
// always names every position, so unlike abstractRecord there is nothing
// left as an implicit wildcard.
func (c *checker) abstractTuple(p syntax.NodeID) pat {
	ent := c.info.Uses[p]
	if ent == 0 {
		return pat{kind: pWild}
	}
	args := make([]pat, 0, len(c.t.Children(p)))
	for _, s := range c.t.Children(p) {
		args = append(args, c.abstract(s))
	}
	return pat{kind: pCtor, key: recordKey(ent), name: c.r.Entities[ent].Name, args: args}
}

func (c *checker) abstractLit(lit syntax.NodeID) pat {
	lit = c.literalNode(lit)
	text := c.t.TokText(c.t.Nodes[lit].Tok)
	if c.t.Kind(lit) == syntax.BoolLit {
		return pat{kind: pCtor, key: text, name: text}
	}
	if v, ok := c.info.Literals[lit]; ok && v.Kind == constInt {
		text = v.Int.String()
	}
	return pat{kind: pLit, key: "l" + text, name: text}
}

// abstractRecord orders the field patterns by declaration (records) or by
// payload position (named variants); unmentioned fields are wildcards.
func (c *checker) abstractRecord(p syntax.NodeID) pat {
	node := c.t.Nodes[p]
	ent := c.info.Uses[p]
	if ent == 0 {
		return pat{kind: pWild}
	}
	subs := map[string]pat{}
	for _, f := range c.t.Children(syntax.NodeID(node.Rhs)) {
		fn := c.t.Nodes[f]
		if fn.Lhs != 0 {
			subs[c.t.TokText(fn.Tok)] = c.abstract(syntax.NodeID(fn.Lhs))
		}
	}
	return c.recordPat(ent, subs)
}

// recordPat assembles a record or named-variant constructor pattern from
// sub-patterns keyed by field name.
func (c *checker) recordPat(ent EntityID, subs map[string]pat) pat {
	e := &c.r.Entities[ent]
	if e.Kind == EntVariant {
		vi := c.r.variant(ent)
		args := wilds(len(vi.Payload))
		for i, name := range vi.Names {
			if s, ok := subs[name]; ok {
				args[i] = s
			}
		}
		return pat{kind: pCtor, key: variantKey(ent), name: e.Name, args: args}
	}
	fields := c.r.typeDecl(ent).Fields
	args := wilds(len(fields))
	for i, f := range fields {
		if s, ok := subs[c.r.Entities[f].Name]; ok {
			args[i] = s
		}
	}
	return pat{kind: pCtor, key: recordKey(ent), name: e.Name, args: args}
}
