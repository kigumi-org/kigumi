package sem

import "kigumi/internal/syntax"

func (r *Result) isEffectPolyField(fld EntityID) bool {
	t := r.Entities[fld].Type
	return r.Types.Kind(t) == KFn && r.Types.Node(t).Flags&fnEffectPoly != 0
}

// recordEffectFacts implements field-argument propagation.
func (c *checker) recordEffectFacts(lit syntax.NodeID, base Place) {
	call, ok := c.info.Calls[lit]
	if !ok || call.Kind != CallRecord {
		return
	}
	var facts []NarrowFact
	for _, entry := range c.t.Children(syntax.NodeID(c.t.Nodes[lit].Rhs)) {
		if c.t.Kind(entry) == syntax.Spread {
			continue
		}
		fld := c.info.Uses[entry]
		if fld == 0 || !c.r.isEffectPolyField(fld) {
			continue
		}
		fact, ok := c.effectFact(syntax.NodeID(c.t.Nodes[entry].Lhs))
		if !ok {
			continue
		}
		fact.Place = c.r.internPlace(c.r.fieldPlace(base, fld))
		facts = append(facts, fact)
	}
	c.addFacts(facts)
}

func (c *checker) effectFact(rhs syntax.NodeID) (NarrowFact, bool) {
	if fn := c.info.Uses[rhs]; fn != 0 && c.r.Entities[fn].Kind == EntFn {
		return NarrowFact{Kind: FactPureField, Mods: c.r.Fn(fn).Declared & (EffPure | EffNoalloc)}, true
	}
	if c.t.Kind(rhs) == syntax.Lambda {
		if cl := c.info.Defs[rhs]; cl != 0 {
			return NarrowFact{Kind: FactPureField, Target: cl}, true
		}
	}
	if p := c.polyParam(rhs); p != 0 {
		return NarrowFact{Kind: FactPureField, Param: p}, true
	}
	return NarrowFact{}, false
}

func (c *checker) knownFieldEffect(callee syntax.NodeID) (NarrowFact, bool) {
	place := c.placeOf(callee)
	if place == 0 {
		return NarrowFact{}, false
	}
	for i := len(c.facts) - 1; i >= 0; i-- {
		if f := c.facts[i]; f.Kind == FactPureField && f.Place == place {
			return f, true
		}
	}
	return NarrowFact{}, false
}

// upgradeFnEffects folds a known effect into the type so the call about to
// be made reports the value's real purity, not the field's unresolved default.
func (c *checker) upgradeFnEffects(ft TypeID, eff Effects) TypeID {
	tt := c.r.Types
	n := tt.Node(ft)
	if n.Kind != KFn || n.Flags&fnEffectPoly == 0 {
		return ft
	}
	return tt.Intern(typeNode{Kind: KFn, Flags: n.Flags | uint16(eff), Elem: n.Elem, Args: n.Args})
}

// stashFieldFact lets callValue add the matching edge later, in place of
// its own unresolved fallback.
func (c *checker) stashFieldFact(n syntax.NodeID, fact NarrowFact) {
	if c.fieldFact == nil {
		c.fieldFact = map[syntax.NodeID]NarrowFact{}
	}
	c.fieldFact[n] = fact
}

// applyFieldFact covers fieldCallbackEdge's call-site resolution, which has
// no callee type to upgrade; callMethod instead upgrades the type directly
// so its unsafe/C-ABI checks see the real flags.
func (c *checker) applyFieldFact(n syntax.NodeID, fact NarrowFact) {
	switch {
	case fact.Param != 0:
		c.deferToParam(fact.Param)
		c.edge(EffectEdge{Kind: EdgeParamCall, Target: fact.Param, Node: n})
	case fact.Target != 0:
		c.edge(EffectEdge{Kind: EdgeCallback, Target: fact.Target, Node: n})
	default:
		c.edge(EffectEdge{Kind: EdgeCallback, Mods: fact.Mods, Node: n})
	}
}
