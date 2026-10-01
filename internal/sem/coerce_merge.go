package sem

import "kigumi/internal/syntax"

// merge joins branch types without an expected type: Never drops
// out, and T merges with T? by lifting.
func (c *checker) merge(nodes []syntax.NodeID, types []TypeID) TypeID {
	tt := c.r.Types
	best := TypeID(0)
	depth := -1
	for _, t := range types {
		t = c.vars.resolve(t)
		if t == TyNever || t == TyPoison {
			continue
		}
		d := 0
		for e, ok := tt.IsOption(t); ok; e, ok = tt.IsOption(e) {
			d++
		}
		if d > depth || d == depth && tt.Kind(best) == KUntyped && tt.Kind(t) != KUntyped {
			best, depth = t, d
		}
	}
	if best == 0 {
		return TyNever
	}
	for i, t := range types {
		t = c.vars.resolve(t)
		if t == TyNever || t == TyPoison {
			continue
		}
		steps, ok := c.coerceSteps(nodes[i], t, best)
		if !ok || !optionOnly(steps) {
			c.errAt(nodes[i], cBranchTypeMismatch, defaultOf(best), defaultOf(t))
			continue
		}
		if len(steps) > 0 {
			c.info.Coerce[nodes[i]] = Coercion{From: t, Steps: steps}
			if steps[0].Kind == CoLiteral {
				c.setType(c.literalNode(nodes[i]), steps[0].To)
			}
		}
	}
	return best
}

func optionOnly(steps []CoStep) bool {
	for _, s := range steps {
		if s.Kind != CoSome && s.Kind != CoLiteral && s.Kind != CoNever {
			return false
		}
	}
	return true
}
