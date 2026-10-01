package sem

import "kigumi/internal/syntax"

// Expected type is unified before the payload so it can resolve typeArgs.
func (c *checker) callVariant(n syntax.NodeID, head headRef, args []syntax.NodeID, want TypeID) TypeID {
	v := head.ent
	vi := c.r.variant(v)
	name := c.r.Entities[v].Name
	c.info.Uses[syntax.NodeID(c.t.Nodes[n].Lhs)] = v
	if len(vi.Payload) == 0 {
		c.errFix(n, c.replaceNode(n, "Write `"+name+"` without parentheses", name), cVariantNoPayload, name, name)
		c.synthArgs(args)
		return TyPoison
	}
	adt := c.r.Entities[v].Parent
	typeArgs := c.adtArgs(n, adt, head.args)
	result := c.r.Types.Named(adt, typeArgs)
	if want != 0 {
		c.unifyWant(n, result, want)
	}
	if len(args) != len(vi.Payload) {
		c.errAt(n, cVariantArity, name, len(vi.Payload), plural(len(vi.Payload)), len(args))
		c.synthArgs(args)
		return result
	}
	subst := map[EntityID]TypeID{}
	for i, p := range c.r.typeDecl(adt).Params {
		subst[p] = typeArgs[i]
	}
	for i, a := range args {
		if c.t.Kind(a) == syntax.Spread {
			c.errAt(a, cSpreadNonVariadic, name)
			continue
		}
		c.checkArg(a, c.r.Types.Subst(vi.Payload[i], subst), 0)
	}
	c.info.Calls[n] = CallInfo{Kind: CallVariant, Callee: v, Inst: typeArgs}
	c.touched = append(c.touched, n)
	return result
}

func (c *checker) callMethod(n, callee syntax.NodeID, args []syntax.NodeID, want TypeID) TypeID {
	node := c.t.Nodes[callee]
	base := syntax.NodeID(node.Lhs)
	name := c.t.TokText(node.Tok)
	recv := c.vars.resolve(c.synth(base))
	tt := c.r.Types
	if recv == TyPoison {
		c.synthArgs(args)
		return TyPoison
	}
	valueType := recv
	if tt.Kind(recv) == KRef {
		valueType = tt.Node(recv).Elem
	}
	if tt.Kind(valueType) == KUntyped {
		if lit := c.literalNode(base); isNumericLiteral(c.t, lit) {
			if resolved, ok := c.resolveLiteralReceiver(valueType, name); ok {
				if recv = c.adopt(lit, resolved); recv == TyPoison {
					c.synthArgs(args)
					return TyPoison
				}
				valueType = recv
			}
		}
	}
	if tt.Kind(valueType) == KNamed {
		if fld := c.r.findField(tt.Node(valueType).Ent, name); fld != 0 {
			ft := c.memberOn(callee, base, recv, name)
			c.setType(callee, ft)
			resolved := c.vars.resolve(ft)
			if fact, ok := c.knownFieldEffect(callee); ok {
				if fact.Param != 0 || fact.Target != 0 {
					c.stashFieldFact(n, fact)
				} else {
					resolved = c.upgradeFnEffects(resolved, fact.Mods)
				}
			} else if p := c.identParam(base); p != 0 {
				c.deferToFieldParam(p, fld)
				c.stashFieldFact(n, NarrowFact{Kind: FactPureField, Param: p})
			}
			return c.callValue(n, resolved, args, want)
		}
	}
	return c.callMethodOn(n, callee, base, recv, valueType, nil, args, want)
}

func (c *checker) callMethodOn(n, callee, base syntax.NodeID, recv, valueType TypeID, typeArgs []TypeID, args []syntax.NodeID, want TypeID) TypeID {
	tt := c.r.Types
	name := c.t.TokText(c.t.Nodes[callee].Tok)
	set := c.r.memberSet(valueType, name)
	if set == 0 {
		if req, sig, ambiguous := c.ifaceMember(callee, valueType, name); req != 0 {
			return c.callRequirement(n, callee, base, recv, req, sig, args, want)
		} else if ambiguous {
			c.synthArgs(args)
			return TyPoison
		}
		c.synthArgs(args)
		if tt.Kind(valueType) == KParam {
			c.errAt(callee, cParamNoMember, valueType, name)
		} else {
			c.errSuggest(callee, cMemberNotFound, name, c.r.memberNames(c.r.namedEnt(valueType)), valueType, name)
		}
		return TyPoison
	}
	members := c.r.Overloads[set].Members
	m := members[0]
	if len(members) == 1 {
		c.r.useEntity(c.f, callee, m)
	} else if m = c.pickOverload(n, set, args, valueType); m == 0 {
		return TyPoison
	}
	info := c.r.Fn(m)
	if info.Recv == RecvNone {
		c.synthArgs(args)
		c.errAt(callee, cMethodAsAssoc, name, c.r.entityName(tt.Node(valueType).Ent), name)
		return TyPoison
	}
	c.info.Uses[callee] = m
	c.checkReceiver(base, recv, m)
	result := c.callFn(n, m, typeArgs, args, want, valueType)
	if info.Recv == RecvMut {
		c.checkReentrantCapture(base, args)
	}
	return result
}

func (c *checker) checkReceiver(base syntax.NodeID, recv TypeID, m EntityID) {
	info := c.r.Fn(m)
	name := c.r.Entities[m].Name
	tt := c.r.Types
	switch info.Recv {
	case RecvMut:
		if tt.Kind(recv) == KRef && tt.Node(recv).Flags&flagMut != 0 {
			return
		}
		if c.indexAssignUnsupported(base) {
			c.errAt(base, cIndexAssignUnsupported)
		} else if !c.isPlace(base) {
			c.errAt(base, cRecvTemporary, name)
		} else if !c.isMutablePlace(base) {
			c.reportRecvNotMutable(base, name)
		} else if root := c.rootOf(base); root != 0 {
			c.checkCapturedMutConflict(base, root)
		}
		c.invalidatePlace(base)
	case RecvMove:
		if tt.Kind(recv) == KRef {
			c.errAt(base, cMoveOutOfBorrow, tt.Node(recv).Elem)
		}
	}
}
