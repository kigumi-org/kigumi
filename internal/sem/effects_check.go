package sem

import "kigumi/internal/syntax"

// checkContracts reports every edge of a declared `pure`/`noalloc` node
// that breaks the contract, and every closure or fn item coerced to a
// contracted fn type whose summary falls short.
func (r *Result) checkContracts(id EntityID) {
	var declared Effects
	if r.Entities[id].Kind != EntClosure {
		declared = r.Fn(id).Declared & (EffPure | EffNoalloc)
	}
	for _, e := range r.edgesOf(id) {
		if e.Kind == EdgeContract {
			r.checkContractEdge(e)
			continue
		}
		for _, mod := range []Effects{EffPure, EffNoalloc} {
			if declared&mod != 0 && !r.edgeAllows(e, mod) {
				r.reportEdge(e, mod)
			}
		}
	}
}

func (r *Result) checkContractEdge(e EffectEdge) {
	target := e.Target
	s := r.summary(target)
	// A field/witness requirement deferred through target's own parameter
	// has no call-site argument to resolve against here, so it
	// can't back an erased fn value's contract even though the summary reads
	// clean (EdgeParamCall/EdgeWitnessCall always allow).
	deferred := r.Entities[target].Kind == EntFn &&
		(len(r.Fn(target).CalledFieldParams) > 0 || len(r.Fn(target).CalledWitnessReqs) > 0)
	for _, mod := range []Effects{EffPure, EffNoalloc} {
		if e.Mods&mod == 0 || (allows(s, mod) && !deferred) {
			continue
		}
		why := r.violationText(target, mod)
		if r.Entities[target].Kind == EntClosure {
			r.errAt(e.File, e.Node, cClosureContract, modName(mod), why)
		} else {
			r.errAt(e.File, e.Node, cFnItemContract, r.Entities[target].Name, modName(mod), why)
		}
	}
}

func (r *Result) violationText(target EntityID, mod Effects) string {
	for _, e := range r.edgesOf(target) {
		if e.Kind == EdgeContract || r.edgeAllows(e, mod) {
			continue
		}
		switch e.Kind {
		case EdgeCall, EdgeWitness:
			return "calls `" + r.Entities[e.Target].Name + "`"
		case EdgeCallback:
			return "calls `" + r.exprName(e.File, e.Node) + "`"
		case EdgeIO:
			return "performs I/O"
		case EdgeMutateCaller:
			return "mutates `" + r.exprName(e.File, e.Node) + "`"
		case EdgeUnsafe:
			return "contains `unsafe`"
		case EdgeAlloc:
			return "allocates"
		case EdgeDrop:
			return "drops `" + r.TypeString(e.Type) + "`"
		}
	}
	if r.Entities[target].Kind == EntFn {
		info := r.Fn(target)
		if len(info.CalledFieldParams) > 0 {
			key := info.CalledFieldParams[0]
			return "calls `" + r.Entities[key.Param].Name + "." + r.Entities[key.Field].Name + "`"
		}
		if len(info.CalledWitnessReqs) > 0 {
			key := info.CalledWitnessReqs[0]
			return "calls `" + r.Entities[key.Param].Name + "." + r.Entities[key.Req].Name + "`"
		}
	}
	if _, fixed := r.fixedSummary(target); fixed {
		return "not declared `" + modName(mod) + "`"
	}
	return "inferred"
}

func (r *Result) reportEdge(e EffectEdge, mod Effects) {
	pure := mod == EffPure
	switch e.Kind {
	case EdgeCall, EdgeWitness:
		name := r.Entities[e.Target].Name
		if r.Entities[e.Target].Kind == EntClosure {
			name = r.exprName(e.File, e.Node)
		}
		switch {
		case pure && r.Entities[r.Entities[e.Target].Parent].Kind == EntInterface:
			r.errAt(e.File, e.Node, cImpureWitness, name, r.entityName(r.Entities[e.Target].Parent))
		case pure:
			r.errAt(e.File, e.Node, cImpureCall, name)
		default:
			r.errAt(e.File, e.Node, cAllocCall, name)
		}
	case EdgeCallback:
		switch {
		case e.Target != 0 && r.Entities[e.Target].Kind == EntClosure:
			r.errAt(e.File, e.Node, cClosureContract, modName(mod), r.violationText(e.Target, mod))
		case e.Target != 0:
			r.errAt(e.File, e.Node, cFnItemContract, r.Entities[e.Target].Name, modName(mod), r.violationText(e.Target, mod))
		case pure:
			r.errAt(e.File, e.Node, cImpureCallback, r.exprName(e.File, e.Node))
		default:
			r.errAt(e.File, e.Node, cAllocCallback, r.exprName(e.File, e.Node))
		}
	case EdgeIO:
		if pure {
			r.errAt(e.File, e.Node, cImpureIO, r.Entities[e.Target].Name)
		} else {
			r.errAt(e.File, e.Node, cAllocCall, r.Entities[e.Target].Name)
		}
	case EdgeMutateCaller:
		r.errAt(e.File, e.Node, cImpureMutation, r.exprName(e.File, e.Node))
	case EdgeUnsafe:
		r.errAt(e.File, e.Node, cImpureUnsafe)
	case EdgeAlloc:
		switch {
		case e.Why == 1:
			r.errAt(e.File, e.Node, cAllocInterpolation)
		case e.Why == 2:
			r.errAt(e.File, e.Node, cAllocVariadic)
		case e.Type != 0:
			r.errAt(e.File, e.Node, cAllocWiden, r.TypeString(e.From), r.TypeString(e.Type))
		default:
			r.errAt(e.File, e.Node, cAllocCall, r.exprName(e.File, e.Node))
		}
	case EdgeDrop:
		if pure {
			r.errAt(e.File, e.Node, cImpureDrop, r.TypeString(e.Type))
		} else {
			r.errAt(e.File, e.Node, cAllocDrop, r.TypeString(e.Type))
		}
	}
}

func (r *Result) exprName(f FileID, n syntax.NodeID) string {
	t := r.tree(f)
	for t.Kind(n) == syntax.CallExpr || t.Kind(n) == syntax.WsCallExpr {
		n = syntax.NodeID(t.Nodes[n].Lhs)
	}
	sp := t.Span(n)
	src := t.File.Src
	if int(sp.End) > len(src) || sp.Start >= sp.End {
		return "this expression"
	}
	return string(src[sp.Start:sp.End])
}
