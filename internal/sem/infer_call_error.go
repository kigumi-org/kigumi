package sem

import "kigumi/internal/syntax"

// errorAsCheck requires E to be a concrete nominal Error type: as[E]
// compares identity the way `is` does.
//
// E must also be Copy: as[E] extracts it from behind a shared &Error
// borrow, so a non-Copy E would let two owners share one unique value.
func (c *checker) errorAsCheck(n syntax.NodeID, fn EntityID, inst []TypeID) {
	e := &c.r.Entities[fn]
	if c.r.Packages[e.Pkg].Path != "std/error" || e.Name != "as" || len(inst) == 0 {
		return
	}
	c.r.langItem(c.f, n, "Context")
	t := c.vars.resolve(inst[0])
	tt := c.r.Types
	if t == TyPoison || tt.Kind(t) == KVar {
		return
	}
	if tt.Kind(t) == KNamed {
		if _, ok := c.r.Implements(t, tt.Iface(tt.errorEnt, nil), c.pkg); ok {
			if !c.r.isCopy(t) {
				c.errAt(n, cMoveOutOfBorrow, t)
			}
			return
		}
	}
	c.errAt(n, cErrorAsType, c.r.TypeString(t))
}
