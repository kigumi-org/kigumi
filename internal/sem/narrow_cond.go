package sem

import (
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// condFacts extracts the true/false facts of a Bool condition.
func (c *checker) condFacts(n syntax.NodeID) (pos, neg []NarrowFact) {
	node := c.t.Nodes[n]
	switch node.Kind {
	case syntax.Paren:
		pos, neg = c.condFacts(syntax.NodeID(node.Lhs))
		c.setType(n, c.info.Types[syntax.NodeID(node.Lhs)])
		return pos, neg
	case syntax.Binary:
		switch c.t.Toks[node.Tok].Kind {
		case token.AndAnd:
			lp, _ := c.condFacts(syntax.NodeID(node.Lhs))
			saved := c.snapshotFacts()
			c.addFacts(lp)
			rp, _ := c.condFacts(syntax.NodeID(node.Rhs))
			c.facts = saved
			c.setType(n, TyBool)
			return append(lp, rp...), nil
		case token.EqEq, token.NotEq:
			c.condLeaf(n)
			return c.noneFacts(n)
		}
	case syntax.Unary:
		if c.t.Toks[node.Tok].Kind == token.Bang {
			pos, neg = c.condFacts(syntax.NodeID(node.Lhs))
			if c.vars.resolve(c.info.Types[syntax.NodeID(node.Lhs)]) == TyBool {
				c.setType(n, TyBool)
				return neg, pos
			}
			c.synth(n)
			return nil, nil
		}
	case syntax.IsExpr:
		c.inCond++
		c.condLeaf(n)
		c.inCond--
		return c.isFacts(n)
	}
	c.condLeaf(n)
	return nil, nil
}

func (c *checker) condLeaf(n syntax.NodeID) TypeID {
	t := c.vars.resolve(c.synth(n))
	if c.r.Types.Kind(t) == KUntyped {
		t = c.adopt(c.literalNode(n), defaultOf(t))
	}
	if t == TyBool || t == TyPoison {
		return t
	}
	if c.isMethodValue(n) {
		c.mismatch(n, TyBool, t)
		return TyPoison
	}
	c.errAt(n, cConditionNotBool, t)
	return TyPoison
}

// isFacts also flags `p is Pattern` as always-true/false when existing
// facts already decide it.
func (c *checker) isFacts(n syntax.NodeID) (pos, neg []NarrowFact) {
	node := c.t.Nodes[n]
	place := c.factPlace(syntax.NodeID(node.Lhs))
	pos, neg = c.patternFacts(syntax.NodeID(node.Rhs), place)
	if len(pos) == 0 {
		return nil, nil
	}
	v := pos[0].Variant
	if is, isNot := c.knownVariant(place, v); is {
		c.errAt(n, cIsAlwaysTrue, c.placeName(syntax.NodeID(node.Lhs)), c.r.Entities[v].Name)
	} else if isNot {
		c.errAt(n, cIsAlwaysFalse, c.placeName(syntax.NodeID(node.Lhs)), c.r.Entities[v].Name)
	}
	return pos, neg
}

func (c *checker) noneFacts(n syntax.NodeID) (pos, neg []NarrowFact) {
	node := c.t.Nodes[n]
	lhs, rhs := syntax.NodeID(node.Lhs), syntax.NodeID(node.Rhs)
	var other syntax.NodeID
	switch {
	case c.isBareNone(rhs):
		other = lhs
	case c.isBareNone(lhs):
		other = rhs
	default:
		return nil, nil
	}
	place := c.factPlace(other)
	none := c.noneVariant()
	if place == 0 || none == 0 {
		return nil, nil
	}
	is := []NarrowFact{{Place: place, Kind: FactIs, Variant: none}}
	isNot := []NarrowFact{{Place: place, Kind: FactIsNot, Variant: none}}
	if c.t.Toks[node.Tok].Kind == token.EqEq {
		return is, isNot
	}
	return isNot, is
}
