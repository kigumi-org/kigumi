package sem

import "kigumi/internal/syntax"

func (c *checker) synthBlock(n syntax.NodeID, want TypeID) TypeID {
	saved := c.scope
	c.pushScope(ScopeBlock, n)
	stmts := c.t.Children(n)
	result := TyUnit
	dead := false
	tailValue := false
	for i, s := range stmts {
		last := i == len(stmts)-1
		if dead {
			c.r.errAt(c.f, s, cUnreachableCode)
			dead = false
		}
		if last && c.t.Kind(s) == syntax.ExprStmt && want != TyUnit && !c.unitTail(syntax.NodeID(c.t.Nodes[s].Lhs)) {
			e := syntax.NodeID(c.t.Nodes[s].Lhs)
			if want == 0 && c.stmtPos == n {
				c.stmtPos = e
			}
			result = c.checkTail(n, e, want)
			c.setType(s, result)
			tailValue = true
			break
		}
		t := c.stmt(s)
		if c.vars.resolve(t) == TyNever {
			dead = true
			result = TyNever
		}
	}
	c.popScope(saved)
	if want != 0 && !tailValue && result != TyNever {
		result = c.coerce(n, TyUnit, want)
	}
	return result
}

// unitTail excludes a bare `for { }` loop: its break
// values may give it a real type.
func (c *checker) unitTail(e syntax.NodeID) bool {
	switch c.t.Kind(e) {
	case syntax.IfExpr:
		return ifExpr(c.t, e).Else == 0
	case syntax.IfLetExpr:
		return ifLet(c.t, e).Else == 0
	case syntax.ForExpr:
		s := forExpr(c.t, e)
		return s.Pattern != 0 || s.Head != 0
	}
	return false
}

func (c *checker) condition(n syntax.NodeID) {
	saved := c.scope
	c.condFacts(n)
	c.scope = saved
}

func (c *checker) synthIf(n syntax.NodeID, want TypeID) TypeID {
	s := ifExpr(c.t, n)
	valuePos := c.stmtPos != n
	saved := c.scope
	entry := c.snapshotFacts()
	c.pushScope(ScopeArm, n)
	pos, neg := c.condFacts(s.Cond)
	c.addFacts(pos)
	then := c.checkBranch(n, s.Then, want, valuePos)
	thenFacts := c.snapshotFacts()
	c.popScope(saved)
	c.facts = append(entry, neg...)
	if s.Else == 0 {
		c.joinBranches(then, thenFacts, TyUnit, c.facts)
		if want != TyUnit && (want != 0 || valuePos) {
			c.errAt(n, cIfValueNeedsElse)
			return TyPoison
		}
		return TyUnit
	}
	els := c.checkBranch(n, s.Else, want, valuePos)
	c.joinBranches(then, thenFacts, els, c.snapshotFacts())
	return c.branchResult(want, valuePos, []syntax.NodeID{s.Then, s.Else}, []TypeID{then, els})
}

// checkBranch: in statement position the body is a statement too, so an
// else-less `if` at its tail is fine.
func (c *checker) checkBranch(n, body syntax.NodeID, want TypeID, valuePos bool) TypeID {
	if !valuePos && want == 0 {
		c.stmtPos = body
	}
	return c.checkTail(n, body, want)
}

func (c *checker) branchResult(want TypeID, valuePos bool, nodes []syntax.NodeID, types []TypeID) TypeID {
	if want != 0 {
		return want
	}
	if valuePos {
		return c.merge(nodes, types)
	}
	for _, t := range types {
		if c.vars.resolve(t) != TyNever {
			return TyUnit
		}
	}
	return TyNever
}

// synthIfLet implements `if let`: a guard may reject a matched
// pattern, so its negative fact carries past the statement only without one.
func (c *checker) synthIfLet(n syntax.NodeID, want TypeID) TypeID {
	s := ifLet(c.t, n)
	valuePos := c.stmtPos != n
	init := c.synth(s.Init)
	if c.r.Types.Kind(c.vars.resolve(init)) == KUntyped {
		init = c.adopt(c.literalNode(s.Init), defaultOf(c.vars.resolve(init)))
	}
	place := c.factPlace(s.Init)
	saved := c.scope
	entry := c.snapshotFacts()
	c.pushScope(ScopeArm, n)
	c.checkPatternOn(s.Pattern, s.Init, init, s.Flags&syntax.FlagMut != 0, c.scope)
	pos, neg := c.patternFacts(s.Pattern, place)
	c.addFacts(pos)
	if s.Guard != 0 {
		gp, _ := c.condFacts(s.Guard)
		c.addFacts(gp)
		neg = nil
	}
	then := c.checkBranch(n, s.Then, want, valuePos)
	thenFacts := c.snapshotFacts()
	c.popScope(saved)
	c.facts = append(entry, neg...)
	if s.Else == 0 {
		c.joinBranches(then, thenFacts, TyUnit, c.facts)
		if want != TyUnit && (want != 0 || valuePos) {
			c.errAt(n, cIfValueNeedsElse)
			return TyPoison
		}
		return TyUnit
	}
	els := c.checkBranch(n, s.Else, want, valuePos)
	c.joinBranches(then, thenFacts, els, c.snapshotFacts())
	return c.branchResult(want, valuePos, []syntax.NodeID{s.Then, s.Else}, []TypeID{then, els})
}
