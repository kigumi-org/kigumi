package sem

import "kigumi/internal/syntax"

// synthBracket implements `a[b]`.
func (c *checker) synthBracket(n syntax.NodeID) TypeID {
	node := c.t.Nodes[n]
	base := syntax.NodeID(node.Lhs)
	items := c.t.Children(syntax.NodeID(node.Rhs))
	head := c.resolveHead(base)
	switch head.kind {
	case headFn:
		if params := c.ownGenerics(head.ent); len(params) > 0 {
			var typeArgs []TypeID
			for i, a := range items {
				typeArgs = append(typeArgs, c.r.resolveExprGenericArg(c.f, c.scope, a, entAt(params, i)))
			}
			c.r.useEntity(c.f, n, head.ent)
			c.info.Uses[n] = head.ent
			sig, _, _ := c.instantiateFn(n, head.ent, typeArgs, 0)
			c.touched = append(c.touched, n)
			return sig
		}
	case headType:
		if len(c.r.typeDecl(head.ent).Params) > 0 {
			c.errAt(n, cTypeAsValue, c.r.Entities[head.ent].Name)
			return TyPoison
		}
	case headSet:
		c.errAt(n, cOverloadValueAmbig, c.r.Overloads[head.set].Name)
		return TyPoison
	case headNone:
		return TyPoison
	}
	recv := c.vars.resolve(c.synth(base))
	if len(items) != 1 {
		c.errAt(n, cIndexCount)
		return TyPoison
	}
	index := c.vars.resolve(c.synth(items[0]))
	return c.indexType(n, items[0], recv, index)
}

func (c *checker) ownGenerics(fn EntityID) []EntityID {
	own := c.r.Fn(fn).TypeParams
	if n := len(c.r.ownerParams(fn)); n > 0 && len(own) >= n && c.r.Fn(fn).Recv != RecvNone {
		return own[n:]
	}
	return own
}

func (c *checker) indexUsize(index TypeID) TypeID {
	tt := c.r.Types
	open := index
	if rng := c.r.langItems["Range"]; tt.Kind(index) == KNamed && rng != 0 && tt.Node(index).Ent == rng {
		open = tt.Node(index).Args[0]
	}
	if _, ok := c.vars.index(open); ok {
		c.vars.unify(open, TyUsize)
	}
	return c.vars.resolve(index)
}

func (c *checker) indexType(n, idx syntax.NodeID, recv, index TypeID) TypeID {
	tt := c.r.Types
	if recv == TyPoison || index == TyPoison {
		return TyPoison
	}
	if tt.Kind(recv) == KRef {
		recv = tt.Node(recv).Elem
	}
	if tt.Kind(index) == KUntyped {
		index = c.adopt(c.literalNode(idx), TyUsize)
	}
	index = c.indexUsize(index)
	rng := c.r.langItems["Range"]
	isRange := rng != 0 && tt.Kind(index) == KNamed && tt.Node(index).Ent == rng && tt.Node(index).Args[0] == TyUsize
	c.info.Calls[n] = CallInfo{Kind: CallIndex}
	switch {
	case recv == TyString || recv == TyBytes:
		if index == TyUsize {
			return TyU8
		}
		if isRange {
			return recv
		}
	case tt.Kind(recv) == KNamed && tt.Node(recv).Ent == tt.arrayEnt:
		if index == TyUsize {
			c.placeOf(n)
			return tt.Node(recv).Args[0]
		}
		if isRange {
			return recv
		}
	case tt.Kind(recv) == KNamed:
		if set := c.r.memberSet(recv, "index"); set != 0 {
			m := c.r.Overloads[set].Members[0]
			sig := tt.Node(c.r.memberSig(recv, m))
			if len(sig.Args) == 1 && sig.Args[0] == index {
				c.info.Calls[n] = CallInfo{Kind: CallIndexWitness, Callee: m}
				c.edge(EffectEdge{Kind: EdgeCall, Target: m, Node: n})
				return sig.Elem
			}
			c.errAt(idx, cIndexType, index)
			return TyPoison
		}
		c.errAt(n, cNotIndexable, recv)
		return TyPoison
	default:
		c.errAt(n, cNotIndexable, recv)
		return TyPoison
	}
	c.errAt(idx, cIndexType, index)
	return TyPoison
}
