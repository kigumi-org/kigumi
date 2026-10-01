package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// isPlaceExpr reports whether n names a local's storage (an identifier, a
// field access, or an array element rooted at one) rather than a
// temporary, so a value taken out of it can be reflected back onto where
// it came from.
func isPlaceExpr(t *syntax.Tree, n syntax.NodeID) bool {
	for {
		switch t.Kind(n) {
		case syntax.Paren, syntax.MemberExpr, syntax.BracketExpr, syntax.MoveExpr:
			n = syntax.NodeID(t.Nodes[n].Lhs)
		case syntax.Ident:
			return true
		default:
			return false
		}
	}
}

// patternBindsOwned reports whether p binds a name by value: a `&`-typed
// binding only aliases the place it matched against (bind, pattern.go), so
// it never justifies marking that place moved.
func (fr *frame) patternBindsOwned(p syntax.NodeID) bool {
	if p == 0 {
		return false
	}
	switch fr.t.Kind(p) {
	case syntax.PatBind:
		return fr.bindsOwned(p)
	case syntax.PatField:
		// A shorthand field (`a`, no `: sub`) has no child node to recurse
		// into below; its bind lives on the field node itself (bind in
		// matchRecord, pattern.go).
		if syntax.NodeID(fr.t.Nodes[p].Lhs) == 0 {
			return fr.bindsOwned(p)
		}
	}
	found := false
	fr.t.EachChild(p, func(ch syntax.NodeID) { found = found || fr.patternBindsOwned(ch) })
	return found
}

// bindsOwned reports whether the entity a PatBind or shorthand PatField
// node p defines binds by value rather than aliasing via `&T`.
func (fr *frame) bindsOwned(p syntax.NodeID) bool {
	typ := fr.in.r.Entity(fr.info.Defs[p]).Type
	return fr.in.r.Types.Kind(typ) != sem.KRef
}

// markMoved overwrites a place's stored value with Moved once its move-only
// contents have been taken elsewhere (a call argument, a pattern binding),
// so the place's own scope exit does not drop the same resource again.
func (fr *frame) markMoved(src syntax.NodeID, v Value) {
	if !fr.in.moveOnly(v) || !isPlaceExpr(fr.t, src) {
		return
	}
	cell, path, ok := fr.resolveMovedPlace(src, v)
	if !ok {
		return
	}
	if len(path) == 0 {
		cell.V = Moved{}
		return
	}
	setPath(cell.V, path, Moved{})
}

// moveStep is one hop of a place expression's access chain, root to leaf: a
// field is a fixed index, a bracket's index is unknown (see resolveMovedPlace).
type moveStep struct {
	bracket bool
	field   int
}

// placeSteps walks n down to its root identifier, collecting the field and
// bracket hops in root-to-leaf order, and returns the root's cell (a `*Ref`
// cell resolves to the referent, with its path folded into base).
func (fr *frame) placeSteps(n syntax.NodeID) (cell *Cell, base []int, steps []moveStep, ok bool) {
	var rev []moveStep
	for {
		node := fr.t.Nodes[n]
		switch node.Kind {
		case syntax.Paren, syntax.MoveExpr:
			n = syntax.NodeID(node.Lhs)
		case syntax.MemberExpr:
			fld := fr.info.Uses[n]
			if fr.in.r.Entity(fld).Kind != sem.EntField {
				return nil, nil, nil, false
			}
			rev = append(rev, moveStep{field: fr.in.r.Field(fld).Index})
			n = syntax.NodeID(node.Lhs)
		case syntax.BracketExpr:
			rev = append(rev, moveStep{bracket: true})
			n = syntax.NodeID(node.Lhs)
		case syntax.Ident:
			ent := fr.info.Uses[n]
			if k := fr.in.r.Entity(ent).Kind; k != sem.EntLocal && k != sem.EntParam {
				return nil, nil, nil, false
			}
			c := fr.cell(ent)
			steps = make([]moveStep, len(rev))
			for i, s := range rev {
				steps[len(rev)-1-i] = s
			}
			if ref, ok := c.V.(*Ref); ok {
				return ref.Cell, append([]int{}, ref.Path...), steps, true
			}
			return c, nil, steps, true
		default:
			return nil, nil, nil, false
		}
	}
}

// resolveMovedPlace never re-evaluates a subexpression: an index could hide
// a side effect, so each bracket hop is found by identity search instead.
func (fr *frame) resolveMovedPlace(n syntax.NodeID, v Value) (*Cell, []int, bool) {
	cell, base, steps, ok := fr.placeSteps(n)
	if !ok {
		return nil, nil, false
	}
	path, ok := matchMoveSteps(walkPath(cell.V, base), steps, v)
	if !ok {
		return nil, nil, false
	}
	return cell, append(base, path...), true
}

func matchMoveSteps(val Value, steps []moveStep, v Value) ([]int, bool) {
	val = deref(val)
	if len(steps) == 0 {
		if coercedMatch(val, v) {
			return nil, true
		}
		return nil, false
	}
	s := steps[0]
	if s.bracket {
		arr, ok := val.(*Array)
		if !ok {
			return nil, false
		}
		for i, e := range arr.Elems {
			if path, ok := matchMoveSteps(e, steps[1:], v); ok {
				return append([]int{i}, path...), true
			}
		}
		return nil, false
	}
	rec, ok := val.(*Record)
	if !ok {
		return nil, false
	}
	path, ok := matchMoveSteps(rec.Fields[s.field], steps[1:], v)
	if !ok {
		return nil, false
	}
	return append([]int{s.field}, path...), true
}

// coercedMatch reports whether v is val, or val wrapped by the checker's
// argument coercions (fr.coerce): CoSome/CoOk wrap in a single-payload
// Variant, CoExistential in a Box, CoBorrow in a Ref. A coerced argument
// still owns the same underlying place, so this lets markMoved find it
// through the wrapping instead of only matching a bare identifier's value.
func coercedMatch(val, v Value) bool {
	for {
		v = deref(v)
		if v == val {
			return true
		}
		switch x := v.(type) {
		case *Variant:
			if len(x.Payload) != 1 {
				return false
			}
			v = x.Payload[0]
		case *Box:
			v = x.V
		default:
			return false
		}
	}
}
