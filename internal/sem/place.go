package sem

import "kigumi/internal/syntax"

func (c *checker) placeOf(n syntax.NodeID) PlaceID {
	if id, ok := c.info.Places[n]; ok {
		return id
	}
	p, ok := c.buildPlace(n)
	if !ok {
		return 0
	}
	id := c.r.internPlace(p)
	c.info.Places[n] = id
	return id
}

func (c *checker) buildPlace(n syntax.NodeID) (Place, bool) {
	node := c.t.Nodes[n]
	switch node.Kind {
	case syntax.Ident:
		ent := c.info.Uses[n]
		if ent == 0 {
			if b, _, ok := c.r.lookup(c.scope, c.t.TokText(node.Tok)); ok {
				ent = b.Ent
			}
		}
		if ent == 0 {
			return Place{}, false
		}
		e := &c.r.Entities[ent]
		if e.Kind != EntLocal && e.Kind != EntParam {
			return Place{}, false
		}
		return c.r.identPlace(ent), true
	case syntax.Paren:
		return c.buildPlace(syntax.NodeID(node.Lhs))
	case syntax.MemberExpr:
		base, ok := c.buildPlace(syntax.NodeID(node.Lhs))
		if !ok {
			return Place{}, false
		}
		fld := c.info.Uses[n]
		if fld == 0 || c.r.Entities[fld].Kind != EntField {
			return Place{}, false
		}
		return c.r.fieldPlace(base, fld), true
	case syntax.BracketExpr:
		base, ok := c.buildPlace(syntax.NodeID(node.Lhs))
		if !ok {
			return Place{}, false
		}
		return Place{Root: base.Root, Fields: base.Fields, Index: true, Deref: base.Deref, Mutable: base.Mutable}, true
	}
	return Place{}, false
}

// only Array[T] has an index-set (E617).
func (c *checker) indexAssignUnsupported(n syntax.NodeID) bool {
	switch c.t.Kind(n) {
	case syntax.Paren, syntax.MemberExpr:
		return c.indexAssignUnsupported(syntax.NodeID(c.t.Nodes[n].Lhs))
	case syntax.BracketExpr:
		base := syntax.NodeID(c.t.Nodes[n].Lhs)
		t := c.vars.resolve(c.info.Types[base])
		if t == 0 || t == TyPoison {
			return false
		}
		tt := c.r.Types
		if tt.Kind(t) == KRef {
			t = tt.Node(t).Elem
		}
		if !(tt.Kind(t) == KNamed && tt.Node(t).Ent == tt.arrayEnt) {
			return true
		}
		return c.indexAssignUnsupported(base)
	}
	return false
}

// isMutablePlace implements the OWN-6 predicate.
func (c *checker) isMutablePlace(n syntax.NodeID) bool {
	p, ok := c.buildPlace(n)
	return ok && p.Mutable
}

func (c *checker) isPlace(n syntax.NodeID) bool {
	_, ok := c.buildPlace(n)
	return ok
}

// Applies only to a local/parameter named directly, not a field or element.
func (c *checker) isIdentPlace(n syntax.NodeID) bool {
	_, ok := c.identPlaceEnt(n)
	return ok
}

func (c *checker) identPlaceEnt(n syntax.NodeID) (EntityID, bool) {
	for c.t.Kind(n) == syntax.Paren {
		n = syntax.NodeID(c.t.Nodes[n].Lhs)
	}
	if c.t.Kind(n) != syntax.Ident {
		return 0, false
	}
	ent := c.info.Uses[n]
	if ent == 0 || (c.r.Entities[ent].Kind != EntLocal && c.r.Entities[ent].Kind != EntParam) {
		return 0, false
	}
	return ent, true
}

func (c *checker) placeName(n syntax.NodeID) string {
	node := c.t.Nodes[n]
	switch node.Kind {
	case syntax.Ident:
		return c.t.TokText(node.Tok)
	case syntax.MemberExpr:
		return c.placeName(syntax.NodeID(node.Lhs)) + "." + c.t.TokText(node.Tok)
	case syntax.BracketExpr:
		return c.placeName(syntax.NodeID(node.Lhs)) + "[..]"
	case syntax.Paren:
		return c.placeName(syntax.NodeID(node.Lhs))
	}
	return "this expression"
}

// mirrors buildPlace's Ident case for an entity with no source node yet (a fresh `let` binding).
func (r *Result) identPlace(ent EntityID) Place {
	e := &r.Entities[ent]
	// a `move self` receiver is owned by the body, so it and its fields are mutable like `let mut`.
	mutable := e.Flags&EfMut != 0 || e.Flags&EfSelf != 0 && e.Flags&EfMove != 0
	deref := r.Types.Kind(e.Type) == KRef
	if deref {
		mutable = r.Types.Node(e.Type).Flags&flagMut != 0
	}
	return Place{Root: ent, Stable: !mutable && !deref, Mutable: mutable, Deref: deref}
}

// mirrors buildPlace's MemberExpr case for building base.field without a source node.
func (r *Result) fieldPlace(base Place, fld EntityID) Place {
	fe := &r.Entities[fld]
	p := Place{Root: base.Root, Fields: append(append([]EntityID(nil), base.Fields...), fld), Index: base.Index, Deref: base.Deref}
	p.Mutable = base.Mutable && fe.Flags&EfMut != 0
	p.Stable = base.Stable && fe.Flags&EfMut == 0
	return p
}

func (c *checker) rootOf(n syntax.NodeID) EntityID {
	p, ok := c.buildPlace(n)
	if !ok {
		return 0
	}
	return p.Root
}
