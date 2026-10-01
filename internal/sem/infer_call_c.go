package sem

import "kigumi/internal/syntax"

// checkArgs has already checked arity and resolved named arguments, so args
// is at least len(params) long and already in parameter order.
func (c *checker) checkCArgs(n syntax.NodeID, name string, sn typeNode, args []syntax.NodeID) TypeID {
	params := sn.Args
	for i, a := range args {
		if i < len(params) {
			c.checkArg(a, params[i], 0)
			continue
		}
		t := c.readThrough(c.vars.resolve(c.synth(a)))
		if c.r.Types.Kind(t) == KUntyped {
			t = c.adopt(c.literalNode(a), defaultOf(t))
		}
		if t != TyPoison && !c.r.Types.IsNumeric(t) && c.r.Types.Kind(t) != KPtr {
			c.errAt(a, cAbiType, c.r.TypeString(t))
		}
	}
	return c.vars.resolve(sn.Elem)
}
