package mir

import "kigumi/internal/sem"

// lifetimeSource finds the existing holder a newly built value that may
// carry a named lifetime should alias: the sole borrow
// argument of a record construction routed through the record's own
// `['a]`, or the call argument whose parameter carries the same lifetime
// entity as the callee's declared return. Neither sem nor MIR ever
// substitutes a lifetime parameter per call site, so this match is
// by entity identity against the callee's own declared signature.
func (c *borrowCheck) lifetimeSource(in Inst) (*borrowInfo, bool) {
	switch in.Op {
	case OpRecord:
		return c.recordLifetimeSource(in)
	case OpCall:
		return c.callLifetimeSource(in)
	}
	return nil, false
}

// recordLifetimeSource requires every borrow argument of the construction
// to be the same holder: sem already rejects a record field holding a
// borrow without a lifetime of the record's own, so any holder argument
// here is legitimate, but a record built from two distinct borrows has no
// single owner MIR can name and is conservatively left unresolved (the
// escape check in checkInst then reports it, per phase-1's scope).
func (c *borrowCheck) recordLifetimeSource(in Inst) (*borrowInfo, bool) {
	var found *borrowInfo
	for _, a := range in.Args {
		b, ok := c.holders[a]
		if !ok {
			continue
		}
		if found != nil && found != b {
			return nil, false
		}
		found = b
	}
	return found, found != nil
}

// callLifetimeSource matches the callee's return lifetime against each of
// its own parameters (offset by one for a receiver, which OpCall always
// lowers as args[0]); the receiver itself is not yet a candidate source.
func (c *borrowCheck) callLifetimeSource(in Inst) (*borrowInfo, bool) {
	if in.Ent == 0 || c.p.R.Entity(in.Ent).Kind != sem.EntFn {
		return nil, false
	}
	info := c.p.R.Fn(in.Ent)
	if info.RetLifetime == 0 {
		return nil, false
	}
	offset := 0
	if info.Recv != sem.RecvNone {
		offset = 1
	}
	for i, prm := range info.Params {
		if c.p.R.Local(prm).Lifetime != info.RetLifetime {
			continue
		}
		if offset+i >= len(in.Args) {
			return nil, false
		}
		b, ok := c.holders[in.Args[offset+i]]
		return b, ok
	}
	return nil, false
}
