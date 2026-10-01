package sem

import "kigumi/internal/syntax"

// Returns 0 after reporting on failure.
func (c *checker) pickOverload(n syntax.NodeID, set OverloadSetID, args []syntax.NodeID, recv TypeID) EntityID {
	os := &c.r.Overloads[set]
	named := c.hasNamedArg(args)
	var cands []EntityID
	for _, m := range os.Members {
		if c.r.visibleFrom(c.r.Entities[m].Vis, c.pkg) && c.arityFits(m, args) {
			cands = append(cands, m)
		}
	}
	switch len(cands) {
	case 0:
		c.errAt(n, cCallArity, os.Name, c.arityList(os.Members), "s", len(args))
		c.synthArgs(args)
		return 0
	case 1:
		return cands[0]
	}
	if c.lambdaNeedsAnnot(args) {
		c.errAt(n, cOverloadLambdaAnnot, os.Name)
		c.synthArgs(nonLambda(c.t, args))
		return 0
	}
	shapesFor, cleanup := c.overloadShapes(args, named)
	defer cleanup()
	var exact, generic []EntityID
	for _, m := range cands {
		if len(c.ownGenerics(m)) > 0 {
			generic = append(generic, m)
		} else if c.trialMatch(m, recv, shapesFor(m)) {
			exact = append(exact, m)
		}
	}
	if len(exact) > 1 {
		exact = preferFixed(c, exact)
	}
	if len(exact) == 0 {
		for _, m := range generic {
			if c.trialMatch(m, recv, shapesFor(m)) {
				exact = append(exact, m)
			}
		}
	}
	switch len(exact) {
	case 1:
		return exact[0]
	case 0:
		c.errAt(n, cOverloadNoMatch, os.Name, c.typesText(shapesFor(cands[0])), c.sigList(cands))
	default:
		c.errAt(n, cOverloadAmbiguous, os.Name, c.sigList(exact))
	}
	return 0
}

func (c *checker) arityFits(m EntityID, args []syntax.NodeID) bool {
	if c.hasNamedArg(args) {
		return c.namedArgsFit(m, args)
	}
	sig := c.r.Types.Node(c.r.Fn(m).Sig)
	if sig.Flags&fnVariadic == 0 {
		return len(args) == len(sig.Args)
	}
	return len(args) >= len(sig.Args)-1
}

func (c *checker) arityList(members []EntityID) string {
	out := ""
	seen := map[string]bool{}
	for _, m := range members {
		sig := c.r.Types.Node(c.r.Fn(m).Sig)
		item := itoa(len(sig.Args))
		if sig.Flags&fnVariadic != 0 {
			item = "at least " + itoa(len(sig.Args)-1)
		}
		if seen[item] {
			continue
		}
		seen[item] = true
		if out != "" {
			out += " or "
		}
		out += item
	}
	return out
}

func (c *checker) lambdaNeedsAnnot(args []syntax.NodeID) bool {
	for _, a := range args {
		if c.t.Kind(a) == syntax.NamedArg {
			a = syntax.NodeID(c.t.Nodes[a].Lhs)
		}
		if c.t.Kind(a) != syntax.Lambda {
			continue
		}
		for _, p := range c.t.Children(syntax.NodeID(c.t.Nodes[a].Lhs)) {
			if param(c.t, p).Type == 0 {
				return true
			}
		}
	}
	return false
}

func nonLambda(t *syntax.Tree, args []syntax.NodeID) []syntax.NodeID {
	var out []syntax.NodeID
	for _, a := range args {
		if t.Kind(a) != syntax.Lambda {
			out = append(out, a)
		}
	}
	return out
}

// Lambdas get an open return type (a fresh var) for the trial match.
func (c *checker) argShapes(args []syntax.NodeID) ([]TypeID, []TypeID) {
	tt := c.r.Types
	var out, trial []TypeID
	for _, a := range args {
		switch c.t.Kind(a) {
		case syntax.Lambda:
			var ps []TypeID
			for _, p := range c.t.Children(syntax.NodeID(c.t.Nodes[a].Lhs)) {
				ps = append(ps, c.r.resolveType(c.f, c.scope, param(c.t, p).Type, posLocal))
			}
			ret := c.vars.fresh(a, "return")
			trial = append(trial, ret)
			out = append(out, tt.Fn(ps, ret, 0, false))
		case syntax.Spread:
			out = append(out, c.vars.resolve(c.synth(syntax.NodeID(c.t.Nodes[a].Lhs))))
		default:
			out = append(out, defaultOf(c.vars.resolve(c.synth(a))))
		}
	}
	return out, trial
}

// Trial unification: binds under m's own fresh type params, then rolls everything back.
func (c *checker) trialMatch(m EntityID, recv TypeID, shapes []TypeID) bool {
	tt := c.r.Types
	mark := c.vars.mark()
	start := c.vars.count()
	sig := tt.Node(c.trialSig(m, recv))
	ok := true
	fixed := len(sig.Args)
	variadic := sig.Flags&fnVariadic != 0
	if variadic {
		fixed--
	}
	for i, s := range shapes {
		param := sig.Args[min(i, len(sig.Args)-1)]
		if variadic && i >= fixed && !(i == len(shapes)-1 && tt.Kind(s) == KNamed && tt.Node(s).Ent == tt.arrayEnt && len(shapes) == fixed+1) {
			param = tt.Node(sig.Args[fixed]).Args[0]
		}
		if !c.vars.unify(s, param) {
			ok = false
			break
		}
	}
	c.vars.rollback(mark)
	c.vars.poisonFrom(start)
	return ok
}

func (c *checker) trialSig(m EntityID, recv TypeID) TypeID {
	sig := c.r.Fn(m).Sig
	if recv != 0 {
		sig = c.r.memberSig(recv, m)
	}
	subst := map[EntityID]TypeID{}
	for _, p := range c.ownGenerics(m) {
		subst[p] = c.vars.fresh(0, c.r.Entities[p].Name)
	}
	return c.r.Types.Subst(sig, subst)
}
