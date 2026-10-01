package sem

import "kigumi/internal/syntax"

// synthTuple checks a tuple literal `(a, b, ...)`.
func (c *checker) synthTuple(n syntax.NodeID, want TypeID) TypeID {
	items := c.t.Children(n)
	ent, ok := c.r.Types.tupleEntity(len(items))
	if !ok {
		c.errAt(n, cTupleArity, len(items))
		for _, it := range items {
			c.synth(it)
		}
		return TyPoison
	}
	typeArgs := c.adtArgs(n, ent, nil)
	result := c.r.Types.Named(ent, typeArgs)
	if want != 0 {
		c.unifyWant(n, result, want)
	}
	info := c.r.typeDecl(ent)
	subst := map[EntityID]TypeID{}
	for i, p := range info.Params {
		subst[p] = typeArgs[i]
	}
	for i, it := range items {
		c.checkArg(it, c.r.Types.Subst(c.r.Entities[info.Fields[i]].Type, subst), 0)
	}
	c.info.Calls[n] = CallInfo{Kind: CallRecord, Callee: ent, Inst: typeArgs}
	c.touched = append(c.touched, n)
	return result
}
