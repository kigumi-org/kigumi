package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// markPatternMoved marks Moved, at src's place, exactly the parts of v that
// pattern p binds by value; a part left untouched (wildcard/literal/`&`-bind)
// keeps its slot in v, so the place's own eventual drop still reaches it.
// Wiping the whole place instead would double-drop the taken part or leak
// the untouched one on a partial bind.
func (fr *frame) markPatternMoved(src, p syntax.NodeID, v Value) {
	if !fr.in.moveOnly(v) || !isPlaceExpr(fr.t, src) {
		return
	}
	cell, path, ok := fr.resolveMovedPlace(src, v)
	if !ok {
		return
	}
	fr.markBoundOwned(p, v, cell, path)
}

func markAt(cell *Cell, path []int) {
	if len(path) == 0 {
		cell.V = Moved{}
		return
	}
	setPath(cell.V, path, Moved{})
}

func extendPath(path []int, i int) []int {
	return append(append([]int{}, path...), i)
}

// markBoundOwned recurses p against the value it already matched, in the
// same field/payload order match/matchRecord/matchCtor/matchTuple use, and
// marks Moved at cell+path for each leaf p takes by value.
func (fr *frame) markBoundOwned(p syntax.NodeID, v Value, cell *Cell, path []int) {
	if p == 0 {
		return
	}
	switch fr.t.Kind(p) {
	case syntax.PatBind:
		if fr.bindsOwned(p) {
			markAt(cell, path)
		}
	case syntax.PatRecord:
		ent := fr.info.Uses[p]
		for _, f := range fr.t.Children(syntax.NodeID(fr.t.Nodes[p].Rhs)) {
			fr.markFieldBoundOwned(f, ent, deref(v), cell, path)
		}
	case syntax.PatCtor:
		fr.markCtorBoundOwned(p, deref(v), cell, path)
	case syntax.PatTuple:
		rec, ok := deref(v).(*Record)
		if !ok {
			return
		}
		for i, s := range fr.t.Children(p) {
			if i < len(rec.Fields) {
				fr.markBoundOwned(s, rec.Fields[i], cell, extendPath(path, i))
			}
		}
	case syntax.PatOr:
		// Alternatives bind the same names at the same types (checker
		// enforced) but not necessarily the same field layout, so which
		// leaf paths were actually taken can't be replayed without
		// re-running match(); fall back to the coarse whole-place mark.
		if fr.patternBindsOwned(p) {
			markAt(cell, path)
		}
	}
}

// markFieldBoundOwned marks a `Type { name }` / `Type { name: sub }` record
// or variant field, resolved by name the same way matchRecord's own field
// lookup does.
func (fr *frame) markFieldBoundOwned(f syntax.NodeID, ent sem.EntityID, v Value, cell *Cell, path []int) {
	fn := fr.t.Nodes[f]
	idx, fv, ok := recordFieldSlot(fr, ent, v, fr.t.TokText(fn.Tok))
	if !ok {
		return
	}
	fieldPath := extendPath(path, idx)
	if sub := syntax.NodeID(fn.Lhs); sub != 0 {
		fr.markBoundOwned(sub, fv, cell, fieldPath)
		return
	}
	if fr.bindsOwned(f) {
		markAt(cell, fieldPath)
	}
}

// recordFieldSlot resolves a record- or variant-pattern field name to its
// positional index and value, mirroring matchRecord's own `get` closure.
func recordFieldSlot(fr *frame, ent sem.EntityID, v Value, name string) (int, Value, bool) {
	switch x := v.(type) {
	case *Record:
		fld := fr.in.r.FindField(ent, name)
		if fld == 0 {
			return 0, nil, false
		}
		idx := fr.in.r.Field(fld).Index
		return idx, x.Fields[idx], true
	case *Variant:
		for i, n := range fr.in.r.Variant(ent).Names {
			if n == name {
				return i, x.Payload[i], true
			}
		}
	}
	return 0, nil, false
}

// markCtorBoundOwned marks a `Ctor(sub, ...)` variant payload or a
// single-field newtype's boxed payload, mirroring matchCtor.
func (fr *frame) markCtorBoundOwned(p syntax.NodeID, v Value, cell *Cell, path []int) {
	subsNode := syntax.NodeID(fr.t.Nodes[p].Rhs)
	if subsNode == 0 {
		return
	}
	subs := fr.t.Children(subsNode)
	ent := fr.info.Uses[p]
	if fr.in.r.Entity(ent).Kind != sem.EntVariant {
		if box, ok := v.(*Box); ok && len(subs) == 1 {
			fr.markBoundOwned(subs[0], box.V, cell, path)
		}
		return
	}
	vr, ok := v.(*Variant)
	if !ok {
		return
	}
	for i, s := range subs {
		if i < len(vr.Payload) {
			fr.markBoundOwned(s, vr.Payload[i], cell, extendPath(path, i))
		}
	}
}
