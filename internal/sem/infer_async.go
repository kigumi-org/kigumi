package sem

import "kigumi/internal/syntax"

// Both back ends run the future's body at this point.
func (c *checker) synthAwait(n syntax.NodeID) TypeID {
	c.dropUnstableFacts()
	t := c.vars.resolve(c.synth(syntax.NodeID(c.t.Nodes[n].Lhs)))
	if e := c.r.Entities[c.fn]; e.Kind != EntFn || c.r.Fn(c.fn).Declared&EffAsync == 0 {
		c.errAt(n, cAwaitOutsideAsync)
	}
	if c.cleanup > 0 {
		c.errAt(n, cCleanupAwait)
	}
	if t == TyPoison {
		return TyPoison
	}
	if node := c.r.Types.Node(t); node.Kind == KNamed && node.Ent == c.r.Types.futureEnt {
		return node.Args[0]
	}
	c.errAt(n, cAwaitNotFuture, c.r.TypeString(t))
	return TyPoison
}

// Matches the lazy Future a direct call to the async fn builds.
func (c *checker) asyncValueFn(sig TypeID) TypeID {
	tt := c.r.Types
	n := tt.Node(sig)
	ret := tt.Named(tt.futureEnt, []TypeID{n.Elem})
	return tt.Fn(n.Args, ret, Effects(n.Flags), false)
}
