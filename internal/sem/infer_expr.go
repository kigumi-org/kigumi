package sem

import (
	"kigumi/internal/syntax"
)

func (c *checker) synthIdent(n syntax.NodeID) TypeID {
	name := c.t.TokText(c.t.Nodes[n].Tok)
	if name == "_" {
		c.errAt(n, cUnderscoreValue)
		return TyPoison
	}
	b, found, ok := c.r.lookup(c.scope, name)
	if !ok {
		if variants := c.r.variantNames[c.pkg][name]; len(variants) > 0 {
			return c.variantValue(n, variants[0], nil)
		}
		return c.unresolved(n, name)
	}
	if b.Ent == 0 {
		return c.fnItemValue(n, b.Set)
	}
	ent := c.r.follow(b.Ent)
	e := &c.r.Entities[ent]
	c.info.Uses[n] = ent
	switch e.Kind {
	case EntLocal, EntParam:
		if e.Flags&EfScript != 0 && c.r.Scopes[found].Kind == ScopeScript && !c.inScript() {
			c.errAt(n, cScriptLocalCapture, name)
			return TyPoison
		}
		e.Flags |= EfUsed
		c.placeOf(n)
		c.noteCapture(n, ent, found)
		return e.Type
	case EntConst:
		c.r.useEntity(c.f, n, ent)
		if c.r.constInfo(ent).State != constDone {
			c.r.evalConstDecl(ent)
		}
		if v := c.r.constInfo(ent).Value; v.Kind == constInt || v.Kind == constFloat {
			c.info.Literals[n] = v
		}
		return e.Type
	case EntFn:
		return c.fnItemValue(n, c.r.Fn(ent).Set)
	case EntVariant:
		return c.variantValue(n, ent, nil)
	case EntImport:
		if e.Detail != 0 {
			return c.fnItemValue(n, OverloadSetID(e.Detail))
		}
		c.errAt(n, cPackageAsValue, name, name)
		return TyPoison
	case EntType, EntAlias, EntInterface:
		c.errAt(n, cTypeAsValue, name)
		return TyPoison
	case EntConstraint:
		c.errAt(n, cConstraintAsType, name)
		return TyPoison
	case EntIntrinsic:
		return c.intrinsicValue(n, ent)
	case EntTypeParam:
		if !c.r.typeParam(ent).IsConst {
			c.errAt(n, cTypeAsValue, name)
			return TyPoison
		}
		local := c.constParamLocal(n, ent)
		if local == 0 {
			c.errAt(n, cTypeAsValue, name)
			return TyPoison
		}
		c.info.Uses[n] = local
		c.r.Entities[local].Flags |= EfUsed
		c.placeOf(n)
		return c.r.Entities[local].Type
	}
	return c.unresolved(n, name)
}

func (c *checker) inScript() bool {
	for s := c.scope; s != 0; s = c.r.Scopes[s].Parent {
		if c.r.Scopes[s].Kind == ScopeScript {
			return true
		}
	}
	return false
}

func (c *checker) unresolved(n syntax.NodeID, name string) TypeID {
	d, ok := c.r.diagAt(c.f, n, cUnresolvedName, name)
	if !ok {
		return TyPoison
	}
	switch name {
	case "host", "print", "eprint":
		d = d.WithNote(c.loc(n), "`"+name+"` exists only in the entry file")
	default:
		if hint := c.scriptLocalHint(name); hint != "" {
			d = d.WithNote(c.loc(n), hint)
		} else if alt := suggest(name, c.visibleNames()); alt != "" {
			d = d.WithHelp("did you mean `" + alt + "`?")
			d.Fixes = append(d.Fixes, c.r.replaceFix(c.f, n, "replace with `"+alt+"`", alt))
		}
	}
	c.r.emitDiag(c.f, d)
	return TyPoison
}

// scriptLocalHint implements ENT-3.
func (c *checker) scriptLocalHint(name string) string {
	for id := 1; id < len(c.r.Entities); id++ {
		e := &c.r.Entities[id]
		if e.Kind == EntLocal && e.File == c.f && e.Flags&EfScript != 0 && e.Name == name {
			return "`" + name + "` is a script-local `let` of `main`; pass it as a parameter"
		}
	}
	return ""
}

// fnItemValue implements CALL-9.
func (c *checker) fnItemValue(n syntax.NodeID, set OverloadSetID) TypeID {
	members := c.r.Overloads[set].Members
	if len(members) != 1 {
		c.errAt(n, cOverloadValueAmbig, c.r.Overloads[set].Name)
		return TyPoison
	}
	fn := members[0]
	c.r.useEntity(c.f, n, fn)
	c.info.Uses[n] = fn
	info := c.r.Fn(fn)
	name := c.r.Entities[fn].Name
	sig := info.Sig
	switch {
	case c.r.Types.Node(sig).Flags&fnVariadic != 0:
		c.errAt(n, cVariadicNoValue, name, name)
		return TyPoison
	case len(c.ownGenerics(fn)) > 0:
		sig, _, _ = c.instantiateFn(n, fn, nil, 0)
		c.touched = append(c.touched, n)
	}
	if info.Declared&EffAsync != 0 {
		sig = c.asyncValueFn(sig)
	}
	return sig
}

func (c *checker) variantValue(n syntax.NodeID, v EntityID, args []TypeID) TypeID {
	c.info.Uses[n] = v
	vi := c.r.variant(v)
	name := c.r.Entities[v].Name
	if len(vi.Payload) > 0 {
		c.errAt(n, cVariantCtorValue, name, name)
		return TyPoison
	}
	adt := c.r.Entities[v].Parent
	return c.r.Types.Named(adt, c.adtArgs(n, adt, args))
}

func (c *checker) intrinsicValue(n syntax.NodeID, ent EntityID) TypeID {
	e := &c.r.Entities[ent]
	if e.Name == "host" {
		host := c.r.langItem(c.f, n, "Host")
		if host == 0 {
			return TyPoison
		}
		c.edge(EffectEdge{Kind: EdgeIO, Target: ent, Node: n})
		return c.r.Types.Named(host, nil)
	}
	c.errAt(n, cIntrinsicNotValue, e.Name)
	return TyPoison
}
