package sem

import (
	"slices"

	"kigumi/internal/syntax"
)

// FieldParamKey names a `pure?` field read through one of a function's own
// parameters for field-argument propagation.
type FieldParamKey struct {
	Param EntityID
	Field EntityID
}

// identParam excludes a closure body, whose summary is trusted once with
// no per-call re-instantiation to defer to, and a `mut` parameter,
// which has no entry-point fact a reassignment could invalidate.
func (c *checker) identParam(n syntax.NodeID) EntityID {
	if c.t.Kind(n) != syntax.Ident || len(c.lambdas) > 0 {
		return 0
	}
	p := c.info.Uses[n]
	if p == 0 || c.r.Entities[p].Kind != EntParam || c.r.Entities[p].Parent != c.fn {
		return 0
	}
	if c.r.Entities[p].Flags&EfMut != 0 {
		return 0
	}
	return p
}

// deferToFieldParam implements field-argument propagation.
func (c *checker) deferToFieldParam(p, field EntityID) {
	info := c.r.Fn(c.fn)
	key := FieldParamKey{Param: p, Field: field}
	if !slices.Contains(info.CalledFieldParams, key) {
		info.CalledFieldParams = append(info.CalledFieldParams, key)
	}
}

// fieldCallbackEdges implements field-argument propagation.
func (c *checker) fieldCallbackEdges(fn EntityID, args []syntax.NodeID) {
	info := c.r.Fn(fn)
	for _, key := range info.CalledFieldParams {
		i := slices.Index(info.Params, key.Param)
		if i < 0 || i >= len(args) || c.t.Kind(args[i]) == syntax.Spread {
			continue
		}
		c.fieldCallbackEdge(args[i], key.Field)
	}
}

// fieldCallbackEdge is effectFact's call-site counterpart: it resolves an
// actual argument expression instead of a record literal's field initializer.
func (c *checker) fieldCallbackEdge(expr syntax.NodeID, field EntityID) {
	if c.t.Kind(expr) == syntax.RecordLit {
		if fact, ok := c.recordLitFieldFact(expr, field); ok {
			c.applyFieldFact(expr, fact)
			return
		}
	} else if place := c.placeOf(expr); place != 0 {
		fp := c.r.internPlace(c.r.fieldPlace(c.r.Places[place], field))
		for i := len(c.facts) - 1; i >= 0; i-- {
			if f := c.facts[i]; f.Kind == FactPureField && f.Place == fp {
				c.applyFieldFact(expr, f)
				return
			}
		}
		if p := c.identParam(expr); p != 0 {
			c.deferToFieldParam(p, field)
			c.edge(EffectEdge{Kind: EdgeParamCall, Target: p, Node: expr})
			return
		}
	}
	c.edge(EffectEdge{Kind: EdgeCallback, Node: expr})
}

// recordLitFieldFact resolves a record literal directly, without a bound
// place (e.g. `useApp(App{view: f})`).
func (c *checker) recordLitFieldFact(lit syntax.NodeID, field EntityID) (NarrowFact, bool) {
	call, ok := c.info.Calls[lit]
	if !ok || call.Kind != CallRecord {
		return NarrowFact{}, false
	}
	for _, entry := range c.t.Children(syntax.NodeID(c.t.Nodes[lit].Rhs)) {
		if c.t.Kind(entry) == syntax.Spread || c.info.Uses[entry] != field {
			continue
		}
		return c.effectFact(syntax.NodeID(c.t.Nodes[entry].Lhs))
	}
	return NarrowFact{}, false
}
