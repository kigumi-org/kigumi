package sem

import "kigumi/internal/syntax"

// Brackets are ambiguous: `obj.items[0](x)` indexes a field then calls the
// element; otherwise they're a method's explicit type arguments.
func (c *checker) callBracketMethod(n, callee syntax.NodeID, args []syntax.NodeID, want TypeID) (TypeID, bool) {
	inner := syntax.NodeID(c.t.Nodes[callee].Lhs)
	if c.t.Kind(inner) != syntax.MemberExpr || c.resolveHead(inner).kind != headValue {
		return 0, false
	}
	tt := c.r.Types
	base := syntax.NodeID(c.t.Nodes[inner].Lhs)
	name := c.t.TokText(c.t.Nodes[inner].Tok)
	items := c.t.Children(syntax.NodeID(c.t.Nodes[callee].Rhs))
	recv := c.vars.resolve(c.synth(base))
	if recv == TyPoison {
		c.synthArgs(args)
		return TyPoison, true
	}
	valueType := recv
	if tt.Kind(recv) == KRef {
		valueType = tt.Node(recv).Elem
	}
	isField := tt.Kind(valueType) == KNamed && c.r.findField(tt.Node(valueType).Ent, name) != 0
	if isField || c.r.memberSet(valueType, name) == 0 {
		ft := c.vars.resolve(c.memberOn(inner, base, recv, name))
		c.setType(inner, ft)
		if len(items) != 1 {
			c.errAt(callee, cIndexCount)
			c.synthArgs(args)
			return TyPoison, true
		}
		index := c.vars.resolve(c.synth(items[0]))
		elem := c.indexType(callee, items[0], ft, index)
		c.setType(callee, elem)
		return c.callValue(n, c.vars.resolve(elem), args, want), true
	}
	var typeArgs []TypeID
	for _, a := range items {
		typeArgs = append(typeArgs, c.r.exprAsType(c.f, c.scope, a))
	}
	return c.callMethodOn(n, inner, base, recv, valueType, typeArgs, args, want), true
}
