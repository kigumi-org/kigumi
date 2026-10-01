package sem

import "kigumi/internal/syntax"

// rejects aliasing a `mut self` call's exclusive receiver access via a mutably-capturing lambda
// argument; scope is deliberately narrow (only a lambda literal, not one
// stored and passed by name), matching the immediate-lambda rule.
func (c *checker) checkReentrantCapture(base syntax.NodeID, args []syntax.NodeID) {
	root := c.rootOf(base)
	if root == 0 {
		return
	}
	for _, a := range args {
		if c.t.Kind(a) == syntax.NamedArg {
			a = syntax.NodeID(c.t.Nodes[a].Lhs)
		}
		if c.t.Kind(a) != syntax.Lambda {
			continue
		}
		closure := c.info.Defs[a]
		if closure == 0 {
			continue
		}
		for _, cap := range c.r.closure(closure).Captures {
			if cap.Local == root && cap.Mode != capCopy {
				c.errAt(a, cReentrantMutCapture, c.r.Entities[root].Name)
				break
			}
		}
	}
}
