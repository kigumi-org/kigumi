package sem

import "kigumi/internal/syntax"

// ComptimeSite is a `comptime { ... }` block.
type ComptimeSite struct {
	File FileID
	Node syntax.NodeID
	Fn   EntityID
}

func (c *checker) synthComptimeBlock(n, inner syntax.NodeID) TypeID {
	t := c.synth(inner)
	if !c.literalType(t) {
		c.errAt(n, cComptimeResultType, c.r.TypeString(t))
	}
	block := c.t.Span(inner)
	c.walkNodes(inner, func(id syntax.NodeID) {
		ent := c.info.Uses[id]
		if ent == 0 {
			return
		}
		e := &c.r.Entities[ent]
		if e.Kind != EntLocal && e.Kind != EntParam {
			return
		}
		if def := c.t.Span(e.Node); e.File != c.f || def.Start < block.Start || def.End > block.End {
			c.errAt(id, cComptimeRuntimeLocal, e.Name)
		}
	})
	c.r.Comptime = append(c.r.Comptime, ComptimeSite{File: c.f, Node: n, Fn: c.fn})
	return t
}

// literalType is what a comptime block may produce: what a literal can spell.
func (c *checker) literalType(t TypeID) bool {
	switch {
	case t == TyBool, t == TyChar, t == TyString, t == TyBytes, t == TyUntypedInt, t == TyUntypedFloat, t == TyPoison:
		return true
	}
	return c.r.Types.IsNumeric(t)
}

func (c *checker) walkNodes(n syntax.NodeID, f func(syntax.NodeID)) {
	f(n)
	c.t.EachChild(n, func(ch syntax.NodeID) { c.walkNodes(ch, f) })
}

// SetLiteral records the evaluated value of a comptime block.
func (r *Result) SetLiteral(f FileID, n syntax.NodeID, v Literal) {
	cv := constValue{Int: v.Int, Float: v.Float, Str: v.Str, Bool: v.Bool, Char: v.Char}
	switch v.Kind {
	case LitInt:
		cv.Kind = constInt
	case LitFloat:
		cv.Kind = constFloat
	case LitString:
		cv.Kind = constString
	case LitBytes:
		cv.Kind = constBytes
	case LitBool:
		cv.Kind = constBool
	case LitChar:
		cv.Kind = constChar
	}
	r.Files[f].Literals[n] = cv
}
