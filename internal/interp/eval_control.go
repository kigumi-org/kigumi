package interp

import (
	"kigumi/internal/syntax"
)

func isErr(c *ctrl, result Value, in *Interp) bool {
	var v Value = result
	if c != nil && c.kind == ctrlReturn {
		v = c.val
	}
	if vr, ok := v.(*Variant); ok {
		return in.r.Entity(vr.V).Name == "Err" && in.r.Entity(vr.V).Parent == in.r.Types.ResultEnt()
	}
	return false
}

func (fr *frame) slotsOf(n syntax.NodeID) map[string]syntax.NodeID {
	out := map[string]syntax.NodeID{}
	names := syntax.SlotNames(fr.t.Kind(n))
	for i, s := range fr.t.Slots(n) {
		out[names[i]] = syntax.NodeID(s)
	}
	return out
}

func (fr *frame) ifExpr(n syntax.NodeID) (Value, *ctrl) {
	s := fr.slotsOf(n)
	cond, c := fr.expr(s["cond"])
	if c != nil {
		return nil, c
	}
	if deref(cond).(Bool) {
		return fr.block(s["then"])
	}
	if s["else"] == 0 {
		return Unit{}, nil
	}
	return fr.expr(s["else"])
}

// ifLet evaluates a (possibly refutable) `if let`: the pattern may
// fail to match, in which case it takes the else-branch like `is` does. A
// temporary scrutinee whose pattern binds nothing is dropped here, same as
// `isExpr`; a place scrutinee that binds by value has that place marked
// moved instead, so its own scope exit does not drop it a second time.
func (fr *frame) ifLet(n syntax.NodeID) (Value, *ctrl) {
	s := fr.slotsOf(n)
	init, c := fr.expr(s["init"])
	if c != nil {
		return nil, c
	}
	matched := fr.match(s["pattern"], init)
	switch {
	case isPlaceExpr(fr.t, s["init"]):
		if matched {
			fr.markPatternMoved(s["init"], s["pattern"], init)
		}
	case fr.in.moveOnly(init) && !fr.patternBinds(s["pattern"]):
		fr.dropDeep(init)
	}
	if matched {
		if g := s["guard"]; g != 0 {
			gv, c := fr.expr(g)
			if c != nil {
				return nil, c
			}
			matched = bool(deref(gv).(Bool))
		}
	}
	if matched {
		return fr.block(s["then"])
	}
	if s["else"] == 0 {
		return Unit{}, nil
	}
	return fr.expr(s["else"])
}

func (fr *frame) matchExpr(n syntax.NodeID) (Value, *ctrl) {
	node := fr.t.Nodes[n]
	lhs := syntax.NodeID(node.Lhs)
	scrutinee, c := fr.expr(lhs)
	if c != nil {
		return nil, c
	}
	for _, arm := range fr.t.Children(syntax.NodeID(node.Rhs)) {
		s := fr.slotsOf(arm)
		// An arm that is not taken leaves its bindings undropped: they are
		// copies of a scrutinee that still owns the value.
		fr.pushScope()
		if !fr.match(s["pattern"], scrutinee) {
			fr.scopes = fr.scopes[:len(fr.scopes)-1]
			continue
		}
		if g := s["guard"]; g != 0 {
			gv, c := fr.expr(g)
			if c != nil {
				return nil, c
			}
			if !deref(gv).(Bool) {
				fr.scopes = fr.scopes[:len(fr.scopes)-1]
				continue
			}
		}
		// The pattern's own binds cloned their part out of scrutinee;
		// mark it moved so the place it came from is not dropped again too.
		fr.markPatternMoved(lhs, s["pattern"], scrutinee)
		v, c := fr.expr(s["body"])
		if c == nil {
			v = fr.transfer(s["body"], v)
		}
		fr.popScope()
		return v, c
	}
	fr.panicAt(n, "no match arm matched")
	return nil, nil
}
