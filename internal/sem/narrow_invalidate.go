package sem

import "kigumi/internal/syntax"

// Unstable places (NAR-2, S8): facts may attach to mutable locals, mutable
// fields and indexed places, and are dropped by the events that could
// change them.

func (c *checker) factPlace(n syntax.NodeID) PlaceID {
	id := c.placeOf(n)
	if id == 0 || c.r.Places[id].Deref || c.r.Places[id].Index {
		return 0
	}
	return id
}

func (c *checker) invalidatePlace(n syntax.NodeID) {
	q, ok := c.buildPlace(n)
	if !ok {
		return
	}
	c.facts = filterFacts(c.facts, func(f NarrowFact) bool {
		return !overlaps(c.r.Places[f.Place], q)
	})
}

func overlaps(p, q Place) bool {
	if p.Root != q.Root {
		return false
	}
	if p.Index || q.Index {
		return true
	}
	n := min(len(p.Fields), len(q.Fields))
	for i := range n {
		if p.Fields[i] != q.Fields[i] {
			return false
		}
	}
	return true
}

func (c *checker) dropUnstableFacts() {
	c.facts = filterFacts(c.facts, func(f NarrowFact) bool {
		return c.r.Places[f.Place].Stable
	})
}

func (c *checker) callInvalidates() {
	c.facts = filterFacts(c.facts, func(f NarrowFact) bool {
		return c.r.Entities[c.r.Places[f.Place].Root].Flags&(EfCapturedMut|EfAddrTaken) == 0
	})
}

func filterFacts(fs []NarrowFact, keep func(NarrowFact) bool) []NarrowFact {
	out := fs[:0:0]
	for _, f := range fs {
		if keep(f) {
			out = append(out, f)
		}
	}
	return out
}
