package sem

import (
	"slices"

	"kigumi/internal/syntax"
)

// Narrowing facts live in c.facts, an ordered list of what's known
// about stable places on the current path; branches copy it, joins intersect it.

func (c *checker) snapshotFacts() []NarrowFact { return slices.Clone(c.facts) }

func (c *checker) addFacts(fs []NarrowFact) {
	for _, f := range fs {
		if !slices.Contains(c.facts, f) {
			c.facts = append(c.facts, f)
		}
	}
}

func intersectFacts(a, b []NarrowFact) []NarrowFact {
	var out []NarrowFact
	for _, f := range a {
		if slices.Contains(b, f) {
			out = append(out, f)
		}
	}
	return out
}

func (c *checker) stablePlace(n syntax.NodeID) PlaceID {
	id := c.placeOf(n)
	if id == 0 || !c.r.Places[id].Stable {
		return 0
	}
	return id
}

func (c *checker) patternFacts(p syntax.NodeID, place PlaceID) (pos, neg []NarrowFact) {
	if place == 0 {
		return nil, nil
	}
	info, ok := c.info.Pats[p]
	if !ok || info.Kind != PatVariant || info.Variant == 0 {
		return nil, nil
	}
	pos = []NarrowFact{{Place: place, Kind: FactIs, Variant: info.Variant}}
	if c.allBindings(p) {
		neg = []NarrowFact{{Place: place, Kind: FactIsNot, Variant: info.Variant}}
	}
	return pos, neg
}

func (c *checker) allBindings(p syntax.NodeID) bool {
	node := c.t.Nodes[p]
	switch node.Kind {
	case syntax.PatWildcard, syntax.PatBind:
		return true
	case syntax.PatCtor:
		for _, s := range c.t.Children(syntax.NodeID(node.Rhs)) {
			if !c.allBindings(s) {
				return false
			}
		}
		return true
	case syntax.PatRecord:
		for _, f := range c.t.Children(syntax.NodeID(node.Rhs)) {
			if sub := syntax.NodeID(c.t.Nodes[f].Lhs); sub != 0 && !c.allBindings(sub) {
				return false
			}
		}
		return true
	case syntax.PatTuple:
		for _, s := range c.t.Children(p) {
			if !c.allBindings(s) {
				return false
			}
		}
		return true
	}
	return false
}

func (c *checker) knownVariant(place PlaceID, v EntityID) (is, isNot bool) {
	for _, f := range c.facts {
		if f.Place != place {
			continue
		}
		switch f.Kind {
		case FactIs:
			if f.Variant == v {
				is = true
			} else {
				isNot = true
			}
		case FactIsNot:
			if f.Variant == v {
				isNot = true
			}
		}
	}
	return is, isNot
}

// A diverging branch contributes nothing to the merged facts.
func (c *checker) joinBranches(thenType TypeID, thenFacts []NarrowFact, elseType TypeID, elseFacts []NarrowFact) {
	thenNever := c.vars.resolve(thenType) == TyNever
	elseNever := c.vars.resolve(elseType) == TyNever
	switch {
	case thenNever && elseNever:
		c.facts = intersectFacts(thenFacts, elseFacts)
	case thenNever:
		c.facts = elseFacts
	case elseNever:
		c.facts = thenFacts
	default:
		c.facts = intersectFacts(thenFacts, elseFacts)
	}
}

func (c *checker) noneVariant() EntityID {
	for _, v := range c.r.typeDecl(c.r.Types.optionEnt).Variants {
		if c.r.Entities[v].Name == "None" {
			return v
		}
	}
	return 0
}
