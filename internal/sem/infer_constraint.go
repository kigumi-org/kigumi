package sem

import "kigumi/internal/syntax"

// constraintCheck implements GEN-3. subst lets a sibling-parameter
// constraint check against this call's actual types, not KParam.
type constraintCheck struct {
	node  syntax.NodeID
	param EntityID
	arg   TypeID
	subst map[EntityID]TypeID
}

func (c *checker) deferConstraints(n syntax.NodeID, params []EntityID, inst []TypeID, subst map[EntityID]TypeID) {
	for i, p := range params {
		if i < len(inst) && len(c.r.typeParam(p).Constraints) > 0 {
			c.constraints = append(c.constraints, constraintCheck{n, p, inst[i], subst})
		}
	}
}

// checkConstraints settles before resolveWitnesses so a directly-constrained
// parameter's failure is recorded once here, not again at its witness site.
func (c *checker) checkConstraints() {
	pending := c.constraints
	c.constraints = nil
	for _, cc := range pending {
		arg := c.vars.resolve(cc.arg)
		if c.r.Types.ContainsVar(arg) {
			c.constraints = append(c.constraints, cc)
			continue
		}
		if c.containsPoison(arg) {
			continue
		}
		for _, con := range c.r.typeParam(cc.param).Constraints {
			con.Type = c.vars.resolve(c.r.Types.Subst(con.Type, cc.subst))
			if con.Kind == CIface {
				if name := c.r.deriveIface(con.Type); name == "Encode" || name == "Decode" {
					if fld, bad := c.r.jsonNestedOptionArg(arg, name); bad {
						c.errAt(cc.node, cJSONNestedOption, arg, name, c.r.Entities[fld].Name)
						c.markUnsatisfied(cc.node, arg, con.Type)
						continue
					}
				}
			}
			if why := c.unsatisfied(arg, con); why != "" {
				c.errAt(cc.node, cConstraintUnsatisfied, arg, c.constraintText(con), why)
				if con.Kind == CIface {
					c.markUnsatisfied(cc.node, arg, con.Type)
				}
			}
		}
	}
	c.resolveWitnesses()
}

func (c *checker) containsPoison(t TypeID) bool {
	if t == TyPoison {
		return true
	}
	n := c.r.Types.Node(t)
	if n.Elem != 0 && c.containsPoison(n.Elem) {
		return true
	}
	for _, a := range n.Args {
		if c.containsPoison(a) {
			return true
		}
	}
	return false
}

func (c *checker) unsatisfied(arg TypeID, con Constraint) string {
	tt := c.r.Types
	switch con.Kind {
	case CIface:
		if tt.Kind(arg) == KParam {
			if c.paramHasIface(arg, con.Type) {
				return ""
			}
			return "add the constraint to `" + c.r.TypeString(arg) + "`"
		}
		if ok, marker := c.markerSatisfied(arg, tt.Node(con.Type).Ent); marker {
			if ok {
				return ""
			}
			return "it cannot be derived"
		}
		res := c.r.conforms(arg, con.Type, c.pkg)
		if res.ok || c.r.requestDerive(arg, con.Type) {
			return ""
		}
		return res.missing
	case CCopy:
		if c.r.isCopy(arg) {
			return ""
		}
		return "it is move-only"
	case CSend:
		if c.r.isSend(arg) {
			return ""
		}
		return "it cannot move to another thread"
	case CSync:
		if c.r.isSync(arg) {
			return ""
		}
		return "it cannot be shared between threads"
	case CFn, CFnMut, CFnOnce:
		return c.fnUnsatisfied(arg, con)
	}
	return ""
}

func (c *checker) markerSatisfied(arg TypeID, iface EntityID) (ok, marker bool) {
	if iface == 0 {
		return false, false
	}
	switch iface {
	case c.r.langItem(0, 0, "Eq"):
		return c.r.hasEq(arg), true
	case c.r.langItem(0, 0, "Hash"):
		return c.r.hasHash(arg), true
	case c.r.langItem(0, 0, "Ord"):
		return c.r.hasOrd(arg), true
	}
	return false, false
}

func (c *checker) paramHasIface(arg, iface TypeID) bool {
	for _, con := range c.r.typeParam(c.r.Types.Node(arg).Ent).Constraints {
		if con.Kind == CIface && con.Type == iface {
			return true
		}
	}
	return false
}

func (c *checker) fnUnsatisfied(arg TypeID, con Constraint) string {
	tt := c.r.Types
	switch tt.Kind(arg) {
	case KFn:
		if c.sameFnShape(arg, con.Type) {
			return ""
		}
		return "signature differs"
	case KClosure:
		info := c.r.closure(tt.Node(arg).Ent)
		if !c.sameFnShape(info.Sig, con.Type) {
			return "signature differs"
		}
		if con.Kind == CFn {
			for _, cap := range info.Captures {
				if cap.Mode == capCell {
					return "the closure assigns `" + c.r.Entities[cap.Local].Name + "`; it is `FnMut`"
				}
			}
		}
		return ""
	case KParam:
		for _, own := range c.r.typeParam(tt.Node(arg).Ent).Constraints {
			if own.Kind >= CFn && own.Kind <= con.Kind && c.sameFnShape(own.Type, con.Type) {
				return ""
			}
		}
		return "add the constraint to `" + c.r.TypeString(arg) + "`"
	}
	return "it is not callable"
}

func (c *checker) constraintText(con Constraint) string {
	switch con.Kind {
	case CCopy:
		return "Copy"
	case CSend:
		return "Send"
	case CSync:
		return "Sync"
	case CFn:
		return "Fn[" + c.r.TypeString(con.Type) + "]"
	case CFnMut:
		return "FnMut[" + c.r.TypeString(con.Type) + "]"
	case CFnOnce:
		return "FnOnce[" + c.r.TypeString(con.Type) + "]"
	}
	return c.r.TypeString(con.Type)
}
