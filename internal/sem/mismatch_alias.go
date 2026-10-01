package sem

import "kigumi/internal/syntax"

// checkAlias is check, but names n's mismatch, if any, after the alias
// spelled at (aliasFile, aliasNode) instead of the bare entity name;
// aliasFile 0 means no hint. The previous hint is saved and
// restored, not zeroed, so a nested checkArg call doesn't clobber an
// outer hint still in scope around n.
func (c *checker) checkAlias(n syntax.NodeID, want TypeID, aliasFile FileID, aliasNode syntax.NodeID) TypeID {
	if aliasFile == 0 {
		return c.check(n, want)
	}
	prevTarget, prevFile, prevNode := c.wantAliasTarget, c.wantAliasFile, c.wantAliasNode
	c.wantAliasTarget, c.wantAliasFile, c.wantAliasNode = n, aliasFile, aliasNode
	t := c.check(n, want)
	c.wantAliasTarget, c.wantAliasFile, c.wantAliasNode = prevTarget, prevFile, prevNode
	return t
}

// mismatchAlias is mismatch with the same override and the same
// save/restore as checkAlias.
func (c *checker) mismatchAlias(n syntax.NodeID, want, got TypeID, aliasFile FileID, aliasNode syntax.NodeID) {
	if aliasFile == 0 {
		c.mismatch(n, want, got)
		return
	}
	prevTarget, prevFile, prevNode := c.wantAliasTarget, c.wantAliasFile, c.wantAliasNode
	c.wantAliasTarget, c.wantAliasFile, c.wantAliasNode = n, aliasFile, aliasNode
	c.mismatch(n, want, got)
	c.wantAliasTarget, c.wantAliasFile, c.wantAliasNode = prevTarget, prevFile, prevNode
}

// checkTail is check for e, an if/match/block-with-statements tail
// position whose value is n's: an alias hint active on n follows
// to e, then reverts, so it cascades through nested tail positions.
func (c *checker) checkTail(n, e syntax.NodeID, want TypeID) TypeID {
	if n == 0 || c.wantAliasTarget != n {
		return c.check(e, want)
	}
	c.wantAliasTarget = e
	t := c.check(e, want)
	c.wantAliasTarget = n
	return t
}

// aliasNameAt reports the alias name spelled at node in file, when node
// is a type-annotation node whose resolved entity is an alias and
// its own recorded type still equals want exactly. The exact-want check
// rejects a variadic parameter's node, whose recorded type is the bare
// element, not the `Array[T]` want actually compares against.
func (r *Result) aliasNameAt(file FileID, node syntax.NodeID, want TypeID) (string, bool) {
	if file == 0 || node == 0 || int(file) >= len(r.Files) {
		return "", false
	}
	f := &r.Files[file]
	if int(node) >= len(f.Uses) || int(node) >= len(f.Types) || f.Types[node] != want {
		return "", false
	}
	use := f.Uses[node]
	if use == 0 || r.Entities[use].Kind != EntAlias {
		return "", false
	}
	return r.Entities[use].Name, true
}

// paramAliasSite returns the file and declared type-annotation node for
// ent's type, or (0, 0) when ent is unknown or has none (a self/variadic/
// witness slot, or a parameter synthesized with no source).
func (r *Result) paramAliasSite(ent EntityID) (FileID, syntax.NodeID) {
	if ent == 0 {
		return 0, 0
	}
	e := &r.Entities[ent]
	if e.File == 0 || e.Node == 0 {
		return 0, 0
	}
	ps := param(r.tree(e.File), e.Node)
	if ps.Type == 0 {
		return 0, 0
	}
	return e.File, ps.Type
}
