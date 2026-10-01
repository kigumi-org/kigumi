package sem

import "slices"

// Effect summaries: a node that declares `pure`/`noalloc` or has
// no body is fixed by its modifiers; every other body starts pure and
// noalloc and loses what its edges deny, iterated to a fixpoint.

func (r *Result) checkEffects() {
	nodes := r.effectNodes()
	r.inferSummaries(nodes)
	for _, id := range nodes {
		r.checkContracts(id)
	}
	r.inferDefault(nodes)
	if r.noDefault {
		r.checkDefaultAllocator()
	}
}

func (r *Result) effectNodes() []EntityID {
	var out []EntityID
	for id := 1; id < len(r.Entities); id++ {
		e := &r.Entities[id]
		if e.Flags&EfPoison != 0 {
			continue
		}
		switch e.Kind {
		case EntFn:
			if r.Fn(EntityID(id)).Body != 0 && r.Entities[e.Parent].Kind != EntInterface {
				out = append(out, EntityID(id))
			}
		case EntContract, EntImplicitMain, EntTest, EntClosure:
			out = append(out, EntityID(id))
		}
	}
	return out
}

func (r *Result) edgesOf(id EntityID) []EffectEdge {
	if r.Entities[id].Kind == EntClosure {
		return r.closure(id).Edges
	}
	return r.Fn(id).Edges
}

func (r *Result) summaryPtr(id EntityID) *EffectSummary {
	if r.Entities[id].Kind == EntClosure {
		return &r.closure(id).Summary
	}
	return &r.Fn(id).Summary
}

func (r *Result) fixedSummary(id EntityID) (EffectSummary, bool) {
	e := &r.Entities[id]
	if e.Kind == EntClosure {
		return EffectSummary{}, false
	}
	if e.Kind != EntFn && e.Kind != EntContract && e.Kind != EntImplicitMain && e.Kind != EntTest {
		return EffectSummary{}, true
	}
	info := r.Fn(id)
	if info.Body == 0 || info.Declared&(EffPure|EffNoalloc) != 0 {
		return EffectSummary{Pure: info.Declared&EffPure != 0, Noalloc: info.Declared&EffNoalloc != 0, Known: true}, true
	}
	return EffectSummary{}, false
}

func (r *Result) summary(id EntityID) EffectSummary {
	if s, ok := r.fixedSummary(id); ok {
		return s
	}
	return *r.summaryPtr(id)
}

func (r *Result) inferSummaries(nodes []EntityID) {
	var inferred []EntityID
	for _, id := range nodes {
		if _, fixed := r.fixedSummary(id); !fixed {
			inferred = append(inferred, id)
			*r.summaryPtr(id) = EffectSummary{Pure: true, Noalloc: true, Known: true}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, id := range inferred {
			s := r.summaryPtr(id)
			pure, noalloc := true, true
			for _, e := range r.edgesOf(id) {
				pure = pure && r.edgeAllows(e, EffPure)
				noalloc = noalloc && r.edgeAllows(e, EffNoalloc)
			}
			if pure != s.Pure || noalloc != s.Noalloc {
				s.Pure, s.Noalloc = pure, noalloc
				changed = true
			}
		}
	}
}

func (r *Result) edgeAllows(e EffectEdge, mod Effects) bool {
	switch e.Kind {
	case EdgeCall, EdgeWitness:
		return allows(r.summary(e.Target), mod)
	case EdgeCallback:
		if e.Param != 0 && !slices.Contains(r.Fn(r.Entities[e.Param].Parent).CalledParams, e.Param) {
			return true
		}
		if e.Target != 0 {
			return allows(r.summary(e.Target), mod)
		}
		return e.Mods&mod != 0
	case EdgeParamCall, EdgeWitnessCall:
		return true
	case EdgeIO:
		return false
	case EdgeMutateCaller, EdgeUnsafe:
		return mod != EffPure
	case EdgeAlloc:
		return mod != EffNoalloc
	case EdgeDrop:
		return allows(r.dropEffect(e.Type), mod)
	}
	return true
}

func allows(s EffectSummary, mod Effects) bool {
	if mod == EffPure {
		return s.Pure
	}
	return s.Noalloc
}

func modName(mod Effects) string {
	if mod == EffPure {
		return "pure"
	}
	return "noalloc"
}
