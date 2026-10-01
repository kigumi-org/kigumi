package sem

import "kigumi/internal/syntax"

func (c *checker) declareLocal(n syntax.NodeID, name string, typ TypeID, mut bool) EntityID {
	e := Entity{Kind: EntLocal, Name: name, Pkg: c.pkg, File: c.f, Node: n, Tok: c.t.Nodes[n].Tok, Parent: c.fn, Type: typ}
	if mut {
		e.Flags |= EfMut
	}
	if c.r.Scopes[c.scope].Kind == ScopeScript {
		e.Flags |= EfScript
	}
	id := c.r.newEntity(e)
	c.r.Entities[id].Detail = c.r.addLocal(LocalInfo{Scope: c.scope})
	c.info.Defs[n] = id
	if name == "nil" {
		c.errAt(n, cNilName)
	}
	if name != "_" {
		c.r.Scopes[c.scope].Names[name] = Binding{Ent: id}
	}
	return id
}

func (c *checker) isResultFn() bool {
	_, _, ok := c.r.Types.IsResult(c.retType)
	return ok
}
