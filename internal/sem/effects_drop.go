package sem

// dropEffect is the Phase 1 subset of EFF-5.
func (r *Result) dropEffect(t TypeID) EffectSummary {
	neutral := EffectSummary{Pure: true, Noalloc: true, Known: true}
	if t == 0 {
		return neutral
	}
	if s, ok := r.dropMemo[t]; ok {
		return s
	}
	r.dropMemo[t] = neutral
	s := r.computeDrop(t, neutral)
	r.dropMemo[t] = s
	return s
}

func (r *Result) computeDrop(t TypeID, neutral EffectSummary) EffectSummary {
	tt := r.Types
	n := tt.Node(t)
	switch n.Kind {
	case KIface:
		return EffectSummary{Known: true}
	case KClosure:
		if n.Ent == 0 {
			return neutral
		}
		s := neutral
		for _, cap := range r.closure(n.Ent).Captures {
			s = meet(s, r.dropEffect(r.Entities[cap.Local].Type))
		}
		return s
	case KNamed:
	default:
		return neutral
	}
	info := r.typeDecl(n.Ent)
	s := neutral
	if info.Form == FormResource && info.Drop != 0 {
		s = meet(s, r.summary(info.Drop))
	}
	if r.langItems["Shared"] == n.Ent {
		s.Noalloc = false
	}
	subst := map[EntityID]TypeID{}
	for i, p := range info.Params {
		if i < len(n.Args) {
			subst[p] = n.Args[i]
		}
	}
	for _, a := range n.Args {
		s = meet(s, r.dropEffect(a))
	}
	for _, f := range info.Fields {
		s = meet(s, r.dropEffect(tt.Subst(r.Entities[f].Type, subst)))
	}
	for _, v := range info.Variants {
		for _, p := range r.variant(v).Payload {
			s = meet(s, r.dropEffect(tt.Subst(p, subst)))
		}
	}
	return s
}

func meet(a, b EffectSummary) EffectSummary {
	return EffectSummary{Pure: a.Pure && b.Pure, Noalloc: a.Noalloc && b.Noalloc, Known: true}
}
