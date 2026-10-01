package sem

import "kigumi/internal/syntax"

// checkPatternOn also sees the scrutinee expression: a `&T` scrutinee or a
// place read through a borrow is matched as T and its non-Copy bindings
// become `&U`.
func (c *checker) checkPatternOn(p, expr syntax.NodeID, scrutinee TypeID, mut bool, scope ScopeID) PatInfo {
	seen := map[string]syntax.NodeID{}
	saved, savedMut := c.patBorrow, c.patBorrowMut
	c.patBorrow, c.patBorrowMut = expr != 0 && c.borrowedPlace(expr), false
	scrutinee = c.vars.resolve(scrutinee)
	if n := c.r.Types.Node(scrutinee); n.Kind == KRef {
		c.patBorrow = true
		c.patBorrowMut = n.Flags&flagMut != 0
		scrutinee = c.vars.resolve(n.Elem)
	}
	info := c.patternInner(p, scrutinee, mut, seen)
	info.Borrow = c.patBorrow
	info.Mut = c.patBorrowMut
	c.patBorrow, c.patBorrowMut = saved, savedMut
	c.info.Pats[p] = info
	return info
}

func (c *checker) borrowedPlace(n syntax.NodeID) bool {
	p, ok := c.buildPlace(n)
	if !ok {
		return false
	}
	if p.Deref {
		return true
	}
	e := &c.r.Entities[p.Root]
	// A borrowed self has EfSelf set and EfMove clear.
	return e.Flags&EfSelf != 0 && e.Flags&EfMove == 0
}

// A mut borrow always binds `&mut T`, Copy or not: writing through it
// (`for x in &mut xs`) must reach the source, which a by-value Copy
// binding would not.
func (c *checker) bindingType(t TypeID) TypeID {
	if c.patBorrow && (c.patBorrowMut || !c.r.isCopy(t)) {
		return c.r.Types.Ref(t, c.patBorrowMut)
	}
	return t
}

func (c *checker) scrutineeOf(t TypeID) TypeID {
	if n := c.r.Types.Node(c.vars.resolve(t)); n.Kind == KRef {
		return c.vars.resolve(n.Elem)
	}
	return t
}
