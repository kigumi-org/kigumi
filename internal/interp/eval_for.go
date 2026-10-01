package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

func (fr *frame) forExpr(n syntax.NodeID) (Value, *ctrl) {
	s := fr.slotsOf(n)
	pattern, head, body := s["pattern"], s["head"], s["body"]
	if pattern == 0 {
		for {
			if head != 0 {
				cond, c := fr.expr(head)
				if c != nil {
					return nil, c
				}
				if !deref(cond).(Bool) {
					return Unit{}, nil
				}
			}
			if c := fr.loopBody(body); c != nil {
				if c.kind == ctrlBreak {
					return c.val, nil
				}
				return nil, c
			}
		}
	}
	iter, c := fr.expr(head)
	if c != nil {
		return nil, c
	}
	ref, borrowed := iter.(*Ref)
	container := deref(iter)
	// An owned head is moved into the loop, the same way a
	// plain call argument is; a lent `self`/`mut self` is exempt the way
	// receiver() exempts it from a method call's own argument passing.
	if !borrowed && !fr.isLentSelf(head) {
		fr.markMoved(head, iter)
	}
	// A borrowed head with no place of its own (e.g. `for x in &f()`) leaves
	// its collection owned by nobody; the loop's temp scope drops it here
	// instead of leaking it.
	if borrowed {
		if operand, ok := borrowOperand(fr.t, head); ok && !isPlaceExpr(fr.t, operand) {
			fr.pushTempScope()
			fr.registerTemp(container)
			defer fr.popTempScope()
		}
	}
	elems := fr.elements(n, container)
	_, isArr := container.(*Array)
	mapRec, isMap := container.(*Record)
	isMap = isMap && fr.isMapRecord(mapRec)
	for i := 0; i < len(elems); i++ {
		fr.pushScope()
		elem := elems[i]
		switch {
		case borrowed && isArr:
			elem = &Ref{Cell: ref.Cell, Path: append(append([]int{}, ref.Path...), i)}
		case borrowed && isMap:
			path := append([]int{}, ref.Path...)
			key := &Ref{Cell: ref.Cell, Path: append(append([]int{}, path...), 0, i)}
			val := &Ref{Cell: ref.Cell, Path: append(append([]int{}, path...), 1, i)}
			elem = &Record{Type: fr.tupleType(mapRec.Type), Fields: []Value{key, val}}
		}
		fr.match(pattern, elem)
		c := fr.loopBody(body)
		fr.popScope()
		if c != nil {
			// The elements not yet reached still own their resources: the
			// container itself was moved into the loop above and its own
			// scope will not drop them.
			if !borrowed {
				for j := i + 1; j < len(elems); j++ {
					fr.dropDeep(elems[j])
				}
			}
			if c.kind == ctrlBreak {
				return Unit{}, nil
			}
			return nil, c
		}
	}
	return Unit{}, nil
}

// borrowOperand unwraps parens down to a `&`/`&mut` expression and returns
// its operand.
func borrowOperand(t *syntax.Tree, n syntax.NodeID) (syntax.NodeID, bool) {
	for {
		switch t.Kind(n) {
		case syntax.Paren:
			n = syntax.NodeID(t.Nodes[n].Lhs)
		case syntax.BorrowExpr:
			return syntax.NodeID(t.Nodes[n].Rhs), true
		default:
			return 0, false
		}
	}
}

// isLentSelf reports whether n names the bare `self`/`mut self` of the
// enclosing method: receiver() shares that value with a call rather than
// moving it, and a `for` head must not move it either.
func (fr *frame) isLentSelf(n syntax.NodeID) bool {
	for fr.t.Kind(n) == syntax.Paren {
		n = syntax.NodeID(fr.t.Nodes[n].Lhs)
	}
	if fr.t.Kind(n) != syntax.Ident {
		return false
	}
	ent := fr.info.Uses[n]
	if ent == 0 {
		return false
	}
	e := fr.in.r.Entity(ent)
	return e.Flags&sem.EfSelf != 0 && e.Flags&sem.EfMove == 0
}

func (fr *frame) loopBody(body syntax.NodeID) *ctrl {
	_, c := fr.block(body)
	if c != nil && c.kind == ctrlContinue {
		return nil
	}
	return c
}

func (fr *frame) elements(n syntax.NodeID, iter Value) []Value {
	switch x := iter.(type) {
	case *Array:
		return append([]Value{}, x.Elems...)
	case Bytes:
		var out []Value
		for _, b := range x {
			out = append(out, Int{V: int64(b), T: sem.TyU8})
		}
		return out
	case *Record:
		if fr.isMapRecord(x) {
			return fr.mapEntries(x)
		}
		lo, hi, inclusive := x.Fields[0].(Int), x.Fields[1].(Int), bool(x.Fields[2].(Bool))
		var out []Value
		for i := lo.V; i < hi.V || (inclusive && i == hi.V); i++ {
			out = append(out, Int{V: i, T: lo.T})
		}
		return out
	}
	fr.panicAt(n, "value is not iterable")
	return nil
}
