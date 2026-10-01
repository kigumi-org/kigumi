package sem

import "kigumi/internal/syntax"

// coerce makes the value of node n (of type got) fit want at a coercion
// site; it returns want, or TyPoison after a mismatch.
func (c *checker) coerce(n syntax.NodeID, got, want TypeID) TypeID {
	if want == 0 {
		return got
	}
	steps, ok := c.coerceSteps(n, got, want)
	if !ok {
		c.mismatch(n, want, got)
		return TyPoison
	}
	if len(steps) > 0 {
		c.info.Coerce[n] = Coercion{From: got, Steps: steps}
		c.touched = append(c.touched, n)
		if steps[0].Kind == CoLiteral {
			c.setType(c.literalNode(n), steps[0].To)
		}
	}
	return want
}

func (c *checker) coerceSteps(n syntax.NodeID, got, want TypeID) ([]CoStep, bool) {
	tt := c.r.Types
	got, want = c.vars.resolve(got), c.vars.resolve(want)
	switch {
	case got == TyPoison || want == TyPoison:
		return nil, true
	case got == TyNever:
		return []CoStep{{CoNever, want}}, true
	case tt.Kind(got) == KUntyped && tt.Kind(want) == KVar:
		if c.vars.unify(got, want) {
			if i, ok := c.vars.index(want); ok {
				c.vars.notePending(i, c.literalNode(n))
			}
			return nil, true
		}
	case c.vars.unifiable(got, want):
		c.vars.unify(got, want)
		return nil, true
	case tt.Kind(got) == KUntyped && tt.IsNumeric(want):
		if c.fitsQuiet(c.literalNode(n), want) {
			return []CoStep{{CoLiteral, want}}, true
		}
		return nil, false
	}
	if elem, ok := tt.IsOption(want); ok {
		if s, ok := c.coerceSteps(n, got, elem); ok {
			return append(s, CoStep{CoSome, want}), true
		}
	}
	if val, _, ok := tt.IsResult(want); ok {
		if _, _, gotIsResult := tt.IsResult(got); !gotIsResult {
			if s, ok := c.coerceSteps(n, got, val); ok {
				return append(s, CoStep{CoOk, want}), true
			}
		}
	}
	if tt.Kind(got) == KUntyped {
		steps, ok := c.coerceSteps(n, defaultOf(got), want)
		if ok && len(steps) > 0 && steps[0].Kind != CoLiteral {
			steps = append([]CoStep{{CoLiteral, defaultOf(got)}}, steps...)
		}
		return steps, ok
	}
	if w := tt.Node(want); w.Kind == KRef && w.Flags&flagMut == 0 && tt.Kind(got) != KRef && c.r.isCopy(got) && c.vars.unifiable(got, w.Elem) {
		c.vars.unify(got, w.Elem)
		return []CoStep{{CoBorrow, want}}, true
	}
	if tt.Kind(want) == KIface && c.r.objectSafe(tt.Node(want).Ent) {
		if res := c.r.conforms(got, want, c.pkg); res.ok {
			for _, w := range res.witnesses {
				if len(c.r.Fn(w).Witnesses) > 0 {
					c.errAt(n, cIfaceBoxWitness, got, want, c.r.Entities[w].Name)
					return []CoStep{{CoExistential, want}}, true
				}
			}
			c.edge(EffectEdge{Kind: EdgeAlloc, Type: want, From: got, Node: n})
			return []CoStep{{CoExistential, want}}, true
		}
	}
	if tt.Kind(want) == KFn {
		return c.coerceToFn(n, got, want)
	}
	return nil, false
}

// coerceToFn erases a function item or plain fn value into the expected fn
// type: shapes must match exactly and contracts must be at least as
// strong.
func (c *checker) coerceToFn(n syntax.NodeID, got, want TypeID) ([]CoStep, bool) {
	tt := c.r.Types
	g, w := tt.Node(got), tt.Node(want)
	if w.Flags&fnCAbi != 0 {
		return c.coerceToCFn(n, got, want)
	}
	if g.Kind == KClosure {
		return c.closureErase(n, g.Ent, want)
	}
	if g.Kind != KFn || g.Flags&(fnVariadic|fnCAbi) != 0 {
		return nil, false
	}
	if !c.sameFnShape(got, want) || (g.Flags^w.Flags)&uint16(EffUnsafe) != 0 {
		return nil, false
	}
	need := Effects(w.Flags) & (EffPure | EffNoalloc)
	if Effects(g.Flags)&need != need {
		if item := c.info.Uses[n]; item != 0 && c.r.Entities[item].Kind == EntFn {
			c.edge(EffectEdge{Kind: EdgeContract, Target: item, Mods: need, Node: n})
		} else {
			c.errAt(n, cFnValueEffects, got, want, effectsText(need&^Effects(g.Flags)))
			return nil, true
		}
	}
	if item := c.info.Uses[n]; item != 0 && c.r.Entities[item].Kind == EntFn {
		return []CoStep{{CoFnItem, want}}, true
	}
	return nil, true
}

// mismatch reports a failed coercion with the most specific wording.
func (c *checker) mismatch(n syntax.NodeID, want, got TypeID) {
	tt := c.r.Types
	want, got = c.vars.resolve(want), c.vars.resolve(got)
	if i, ok := c.vars.index(want); ok {
		if c.vars.pending[i] == 0 {
			return
		}
		want = TyI64
		if c.vars.pending[i] == 2 {
			want = TyF64
		}
	}
	if want == TyPoison || got == TyPoison {
		return
	}
	if tt.Kind(got) == KUntyped && !tt.IsNumeric(want) {
		got = c.adopt(c.literalNode(n), defaultOf(got))
	}
	if tt.Kind(want) == KIface && !c.r.objectSafe(tt.Node(want).Ent) {
		return
	}
	if tt.Kind(want) == KIface {
		if res := c.r.conforms(got, want, c.pkg); res.generic {
			c.errAt(n, cIfaceCoerceUnsat, got, want, res.missing)
			c.vars.poison(got)
			c.vars.poison(want)
			return
		}
	}
	if _, _, ok := tt.IsResult(got); ok {
		if _, _, ok := tt.IsResult(want); ok {
			c.errAt(n, cResultAlready, got)
			return
		}
	}
	if tt.Kind(want) == KIface && tt.Node(want).Ent == tt.errorEnt {
		c.errAt(n, cErrorNotConform, got)
		return
	}
	if call, ok := c.info.Calls[n]; ok && call.Kind == CallMethodValue && tt.Kind(want) != KFn {
		c.errAt(n, cMethodNeedsCall, c.r.Entities[call.Callee].Name, c.r.Entities[call.Callee].Name)
		return
	}
	if tt.Kind(got) == KUntyped && tt.IsNumeric(want) {
		c.checkLiteralFits(c.literalNode(n), want)
		return
	}
	ws, gs := c.r.mismatchLabels(c.vars.literalLabel(want), c.vars.literalLabel(got))
	if n == c.wantAliasTarget {
		if alias, ok := c.r.aliasNameAt(c.wantAliasFile, c.wantAliasNode, want); ok {
			ws = alias
		}
	}
	c.errAt(n, cTypeMismatch, ws, gs)
	c.vars.poison(got)
	c.vars.poison(want)
}
