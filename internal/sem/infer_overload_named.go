package sem

import "kigumi/internal/syntax"

func (c *checker) candidateFixed(m EntityID) int {
	sig := c.r.Types.Node(c.r.Fn(m).Sig)
	fixed := len(sig.Args)
	if sig.Flags&fnVariadic != 0 {
		fixed--
	}
	return fixed
}

func (c *checker) namedArgsFit(m EntityID, args []syntax.NodeID) bool {
	sig := c.r.Types.Node(c.r.Fn(m).Sig)
	fixed := c.candidateFixed(m)
	if sig.Flags&fnVariadic != 0 {
		if len(args) < fixed {
			return false
		}
	} else if len(args) != len(sig.Args) {
		return false
	}
	return c.layoutNamed(args, c.paramNames(c.r.Fn(m).Params, fixed)).ok()
}

// Callers only use this once namedArgsFit(m, args) holds, so the layout always succeeds.
func (c *checker) orderedFor(m EntityID, args []syntax.NodeID) []syntax.NodeID {
	names := c.paramNames(c.r.Fn(m).Params, c.candidateFixed(m))
	return c.layoutNamed(args, names).args()
}

// Positional calls share one shape list; named calls recompute per candidate
// since overloads may order parameters differently, cached so
// each candidate is shaped once.
func (c *checker) overloadShapes(args []syntax.NodeID, named bool) (func(EntityID) []TypeID, func()) {
	if !named {
		shapes, trial := c.argShapes(args)
		return func(EntityID) []TypeID { return shapes },
			func() {
				for _, t := range trial {
					c.vars.poison(t)
				}
			}
	}
	cache := map[EntityID][]TypeID{}
	var allTrial []TypeID
	get := func(m EntityID) []TypeID {
		if sh, ok := cache[m]; ok {
			return sh
		}
		sh, tr := c.argShapes(c.orderedFor(m, args))
		cache[m] = sh
		allTrial = append(allTrial, tr...)
		return sh
	}
	return get, func() {
		for _, t := range allTrial {
			c.vars.poison(t)
		}
	}
}
