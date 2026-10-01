package sem

import "kigumi/internal/syntax"

func (c *checker) callIntrinsic(n syntax.NodeID, ent EntityID, args []syntax.NodeID) TypeID {
	name := c.r.Entities[ent].Name
	switch name {
	case "panic":
		if len(args) > 1 {
			c.errAt(n, cPanicArgs)
			c.synthArgs(args)
		} else if len(args) == 1 {
			c.check(args[0], TyString)
		}
		c.edge(EffectEdge{Kind: EdgePanic, Node: n})
		c.info.Calls[n] = CallInfo{Kind: CallIntrinsic, Callee: ent}
		return TyNever
	case "print", "eprint":
		if len(args) != 1 {
			c.errAt(n, cPrintArg, name)
			c.synthArgs(args)
			return TyUnit
		}
		c.check(args[0], TyString)
		c.edge(EffectEdge{Kind: EdgeIO, Target: ent, Node: n})
		c.info.Calls[n] = CallInfo{Kind: CallIntrinsic, Callee: ent}
		return TyUnit
	}
	c.synthArgs(args)
	c.errAt(n, cNotCallable, name)
	return TyPoison
}
