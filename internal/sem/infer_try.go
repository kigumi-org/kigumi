package sem

import "kigumi/internal/syntax"

// synthFail checks `fail e` against the enclosing function's error type;
// the value never returns.
func (c *checker) synthFail(n syntax.NodeID) TypeID {
	operand := syntax.NodeID(c.t.Nodes[n].Lhs)
	if c.cleanup > 0 {
		c.errAt(n, cCleanupControlFlow, "`fail`")
	}
	_, errType, ok := c.r.Types.IsResult(c.retType)
	if !ok {
		if c.lambdaRetUnknown() {
			c.errAt(n, cTryInLambdaUnknownRet)
		} else if c.retType != TyPoison {
			c.errAt(n, cFailOutsideResult, c.retType)
		}
		c.synth(operand)
		return TyNever
	}
	if operand == 0 {
		c.errAt(n, cReturnMissingValue, errType)
		return TyNever
	}
	c.check(operand, errType)
	return TyNever
}

// synthTry checks postfix `?`.
func (c *checker) synthTry(n syntax.NodeID) TypeID {
	operand := syntax.NodeID(c.t.Nodes[n].Lhs)
	tt := c.r.Types
	got := c.vars.resolve(c.synth(operand))
	if c.cleanup > 0 {
		c.errAt(n, cCleanupControlFlow, "`?`")
	}
	if tt.Kind(got) == KUntyped {
		got = c.adopt(c.literalNode(operand), defaultOf(got))
	}
	if got == TyPoison {
		return TyPoison
	}
	borrowed := tt.Kind(got) == KRef
	if borrowed {
		got = tt.Node(got).Elem
	}
	if _, ok := tt.IsOption(got); ok {
		c.errAt(n, cTryOnOption)
		return TyPoison
	}
	val, e, ok := tt.IsResult(got)
	if !ok {
		c.errAt(n, cTryNotResult, got)
		return TyPoison
	}
	if borrowed && !c.r.isCopy(val) {
		c.errAt(n, cMoveOutOfBorrow, val)
	}
	_, want, retOK := tt.IsResult(c.retType)
	if !retOK {
		if c.lambdaRetUnknown() {
			c.errAt(n, cTryInLambdaUnknownRet)
		} else if c.retType != TyPoison {
			c.errAt(n, cTryOutsideResult, c.retType)
		}
		return val
	}
	c.info.Calls[n] = CallInfo{Kind: CallTry}
	if e == want || e == TyPoison || want == TyPoison {
		return val
	}
	if tt.Kind(e) == KVar && c.vars.unify(e, want) {
		return val
	}
	if want == tt.errorType() && c.r.conforms(e, want, c.pkg).ok {
		c.info.Calls[n] = CallInfo{Kind: CallTry, Inst: []TypeID{e}}
		c.edge(EffectEdge{Kind: EdgeAlloc, Type: want, From: e, Node: n})
		return val
	}
	c.errAt(n, cTryErrorMismatch, e, want)
	return val
}
