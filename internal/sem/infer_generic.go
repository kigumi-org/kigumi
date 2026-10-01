package sem

import "kigumi/internal/syntax"

func (c *checker) hintFnParams(own []EntityID, inst []TypeID, subst map[EntityID]TypeID) {
	for i, p := range own {
		idx, ok := c.vars.index(inst[i])
		if !ok {
			continue
		}
		for _, con := range c.r.typeParam(p).Constraints {
			if con.Kind == CFn || con.Kind == CFnMut || con.Kind == CFnOnce {
				c.vars.hint[idx] = c.r.Types.Subst(con.Type, subst)
			}
		}
	}
}

// instantiateFn returns bad=true when the explicit type argument count was
// wrong, so own's instantiation is made up.
func (c *checker) instantiateFn(n syntax.NodeID, fn EntityID, typeArgs []TypeID, recv TypeID) (TypeID, []TypeID, bool) {
	info := c.r.Fn(fn)
	tt := c.r.Types
	subst := map[EntityID]TypeID{}
	ownerParams := c.r.ownerParams(fn)
	own := info.TypeParams
	if recv != 0 && len(ownerParams) > 0 && len(own) >= len(ownerParams) && own[0] == ownerParams[0] {
		rn := tt.Node(recv)
		var recvArgs []TypeID
		for i, p := range ownerParams {
			if i < len(rn.Args) {
				subst[p] = rn.Args[i]
				recvArgs = append(recvArgs, rn.Args[i])
			}
		}
		if len(info.RecvParams) > 0 {
			c.deferConstraints(n, info.RecvParams, recvArgs, subst)
		}
		own = own[len(ownerParams):]
	}
	bad := false
	if len(typeArgs) > 0 && len(typeArgs) != len(own) {
		c.errAt(n, cTypeArgCount, c.r.Entities[fn].Name, len(own), plural(len(own)), len(typeArgs))
		typeArgs = nil
		bad = true
	}
	var inst []TypeID
	for i, p := range own {
		var t TypeID
		switch {
		case i < len(typeArgs):
			t = typeArgs[i]
		case c.r.typeParam(p).IsLifetime:
			// A fresh var lets a lifetime unify from an
			// argument that already carries one; the own-entity fallback
			// covers a return-only `'a` that occurs in no argument.
			t = c.vars.freshLifetime(n, p, p)
		default:
			t = c.vars.fresh(n, c.r.Entities[p].Name)
		}
		subst[p] = t
		inst = append(inst, t)
	}
	if len(inst) > 0 && !bad {
		c.r.Instances[fn] = append(c.r.Instances[fn], Instance{Args: inst, File: c.f, Node: n, In: c.fn})
		c.deferConstraints(n, own, inst, subst)
		c.hintFnParams(own, inst, subst)
	}
	return tt.Subst(info.Sig, subst), inst, bad
}
