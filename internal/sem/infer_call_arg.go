package sem

import "kigumi/internal/syntax"

// paramEnt is the declared parameter entity for the alias hint, or 0 when there is none.
func (c *checker) checkArg(a syntax.NodeID, param TypeID, paramEnt EntityID) {
	aliasFile, aliasNode := c.r.paramAliasSite(paramEnt)
	param = c.vars.resolve(param)
	if !c.r.Types.ContainsVar(param) || c.t.Kind(a) == syntax.Lambda {
		c.checkAlias(a, param, aliasFile, aliasNode)
		if c.info.Types[a] == TyPoison {
			c.vars.poison(param)
		}
		return
	}
	var got TypeID
	if c.t.Kind(a) == syntax.BorrowExpr {
		got = c.vars.resolve(c.setType(a, c.synthBorrow(a, param)))
	} else {
		got = c.vars.resolve(c.synth(a))
	}
	if got == TyPoison {
		c.vars.poison(param)
		return
	}
	steps, ok := c.coerceSteps(a, got, param)
	if !ok {
		c.mismatchAlias(a, param, got, aliasFile, aliasNode)
		return
	}
	if len(steps) > 0 {
		c.info.Coerce[a] = Coercion{From: got, Steps: steps}
		c.touched = append(c.touched, a)
	}
}

// entAt is params[i], or 0 when i is out of range (e.g. no source declaration).
func entAt(params []EntityID, i int) EntityID {
	if i < 0 || i >= len(params) {
		return 0
	}
	return params[i]
}

func (c *checker) unifyWant(n syntax.NodeID, ret, want TypeID) {
	tt := c.r.Types
	var steps []CoStep
	for {
		if c.vars.unify(ret, want) {
			break
		}
		if elem, ok := tt.IsOption(want); ok {
			steps = append([]CoStep{{CoSome, want}}, steps...)
			want = elem
			continue
		}
		if val, _, ok := tt.IsResult(want); ok {
			steps = append([]CoStep{{CoOk, want}}, steps...)
			want = val
			continue
		}
		return
	}
	if len(steps) > 0 {
		c.info.Coerce[n] = Coercion{From: ret, Steps: steps}
	}
}
