package sem

import "kigumi/internal/syntax"

// dlCall reports true when it typed the call itself (std/dl).
func (c *checker) dlCall(n syntax.NodeID, fn EntityID, sig TypeID, inst []TypeID, recv TypeID, args []syntax.NodeID, want TypeID) (TypeID, bool) {
	e := &c.r.Entities[fn]
	info := c.r.Fn(fn)
	if info.Owner == 0 || c.r.Packages[e.Pkg].Path != "std/dl" {
		return 0, false
	}
	tt := c.r.Types
	owner := c.r.Entities[info.Owner].Name
	switch {
	case owner == "Library" && e.Name == "get":
		// T is usually fixed by the expected type, so check it after args/result unify.
		ret, order := c.checkArgs(n, e.Name, sig, args, inst, want, info.Params)
		if t := c.vars.resolve(inst[0]); t != TyPoison && tt.Kind(t) != KVar && !tt.IsCFn(t) {
			c.errAt(n, cDlSymbolType, c.r.TypeString(t))
		}
		c.info.Calls[n] = CallInfo{Kind: CallMethod, Callee: fn, Inst: inst, Recv: info.Recv, ArgOrder: order}
		c.touched = append(c.touched, n)
		return ret, true
	case owner == "Symbol" && e.Name == "call":
		rt := c.vars.resolve(recv)
		if tt.Kind(rt) == KRef {
			rt = c.vars.resolve(tt.Node(rt).Elem)
		}
		rn := tt.Node(rt)
		if rn.Kind != KNamed || len(rn.Args) != 1 {
			c.synthArgs(args)
			return TyPoison, true
		}
		sig := c.vars.resolve(rn.Args[0])
		if !tt.IsCFn(sig) {
			if sig != TyPoison {
				c.errAt(n, cDlSymbolType, c.r.TypeString(sig))
			}
			c.synthArgs(args)
			return TyPoison, true
		}
		ret, order := c.checkArgs(n, "call", sig, args, nil, want, nil)
		effArgs := args
		if order != nil {
			effArgs = order
		}
		c.info.Calls[n] = CallInfo{Kind: CallMethod, Callee: fn, Recv: info.Recv, Variadic: c.variadicForm(effArgs, sig), ArgOrder: order}
		c.touched = append(c.touched, n)
		return ret, true
	}
	return 0, false
}
