package mir

import (
	"strconv"

	"kigumi/internal/sem"
)

// places gives every chain of field reads rooted at a named local one identity,
// so that two reads of the same field are one place to ownership analysis
// rather than two unrelated temporaries.
type places struct {
	key  map[LocalID]string
	rep  map[string]LocalID
	name map[LocalID]string
	// children lists a root's own direct field-place representatives, by
	// field index, so own.go can tell which of its fields are still valid.
	children map[LocalID]map[int]LocalID
}

// findPlaces keys each projection temporary and elects the first temporary of a
// key as its representative. Blocks are revisited until stable because a chain
// can cross a back edge.
func findPlaces(p *Program, f *Func) *places {
	pl := &places{key: map[LocalID]string{}, rep: map[string]LocalID{}, name: map[LocalID]string{},
		children: map[LocalID]map[int]LocalID{}}
	t := p.R.Tree(f.File)
	for changed := true; changed; {
		changed = false
		for bi := range f.Blocks {
			for _, in := range f.Blocks[bi].Insts {
				if (in.Op != OpField && in.Op != OpFieldMove) || in.Dst == 0 || pl.key[in.Dst] != "" {
					continue
				}
				base, ok := pl.base(f, in)
				if !ok {
					continue
				}
				key := base + "." + strconv.Itoa(in.Index)
				pl.key[in.Dst] = key
				if _, seen := pl.rep[key]; !seen {
					pl.rep[key] = in.Dst
					sp := t.Span(in.Node)
					pl.name[in.Dst] = string(t.File.Src[sp.Start:sp.End])
				}
				changed = true
			}
		}
	}
	for bi := range f.Blocks {
		for _, in := range f.Blocks[bi].Insts {
			if (in.Op != OpField && in.Op != OpFieldMove) || pl.canon(in.Dst) != in.Dst {
				continue
			}
			root := pl.canon(in.Args[0])
			if pl.children[root] == nil {
				pl.children[root] = map[int]LocalID{}
			}
			pl.children[root][in.Index] = in.Dst
		}
	}
	return pl
}

// base names the container a field instruction reads from: another projection,
// or a named local. Temporaries of unknown provenance have no place.
func (pl *places) base(f *Func, in Inst) (string, bool) {
	if k := pl.key[in.Args[0]]; k != "" {
		return k, true
	}
	if f.Locals[in.Args[0]].Ent == 0 {
		return "", false
	}
	return strconv.Itoa(int(in.Args[0])), true
}

func (pl *places) isPlace(l LocalID) bool { return pl.key[l] != "" }

func (pl *places) canon(l LocalID) LocalID {
	if k := pl.key[l]; k != "" {
		return pl.rep[k]
	}
	return l
}

// stored names the place a field store writes back, which owns a value again.
func (pl *places) stored(f *Func, in Inst) (LocalID, bool) {
	base, ok := pl.base(f, in)
	if !ok {
		return 0, false
	}
	l, ok := pl.rep[base+"."+strconv.Itoa(in.Index)]
	return l, ok
}

// consumesArg reports whether argument i hands ownership over. It mirrors the
// backends' lending rule: a `self`/`mut self` receiver, the callee of an
// indirect call and every argument of a runtime primitive are lent, not
// moved, except `await`'s future (it runs the future's body and discards it)
// and a resource's own registered `drop`, which always consumes `self`.
func (p *Program) consumesArg(in *Inst, i int) bool {
	switch in.Op {
	case OpCall:
		e := p.R.Entity(in.Ent)
		if e.Kind != sem.EntFn {
			return false
		}
		info := p.R.Fn(in.Ent)
		if i == 0 && p.isOwnDrop(in.Ent, info) {
			return true
		}
		if info.Body == 0 && info.Abi == "" && e.Flags&sem.EfStd != 0 {
			return false
		}
		// C code releases nothing: every argument of a foreign call is lent.
		if info.Abi != "" || info.Naked {
			return false
		}
		return i != 0 || info.Recv == sem.RecvNone || info.Recv == sem.RecvMove
	case OpCallValue:
		// A witness call names its requirement: a `self` / `mut self`
		// receiver is lent, as in a direct method call.
		if in.Ent != 0 && i == 1 {
			recv := p.R.Fn(in.Ent).Recv
			return recv == sem.RecvNone || recv == sem.RecvMove
		}
		return i != 0
	case OpBuiltin:
		return in.Str == "await"
	}
	return false
}
