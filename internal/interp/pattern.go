package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// match tests a pattern against a value and binds its names.
func (fr *frame) match(p syntax.NodeID, v Value) bool {
	node := fr.t.Nodes[p]
	switch node.Kind {
	case syntax.PatWildcard:
		return true
	case syntax.PatBind:
		// v is kept intact (not deref'd): bind() itself decides, from the
		// binding's own type, whether to keep a *Ref alive or copy through it.
		fr.bind(fr.info.Defs[p], v)
		return true
	case syntax.PatLit:
		lit, c := fr.expr(syntax.NodeID(node.Lhs))
		if c != nil {
			return false
		}
		return valueEqual(lit, deref(v))
	case syntax.PatCtor:
		return fr.matchCtor(p, deref(v))
	case syntax.PatRecord:
		return fr.matchRecord(p, deref(v))
	case syntax.PatTuple:
		return fr.matchTuple(p, deref(v))
	case syntax.PatOr:
		for _, alt := range fr.t.Children(p) {
			if fr.match(alt, v) {
				return true
			}
		}
		return false
	case syntax.PatRange:
		return fr.matchRange(p, deref(v))
	}
	return false
}

// matchRange tests `lo..hi` / `lo..=hi` against an integer scrutinee.
func (fr *frame) matchRange(p syntax.NodeID, v Value) bool {
	node := fr.t.Nodes[p]
	lo, c := fr.expr(syntax.NodeID(node.Lhs))
	if c != nil {
		return false
	}
	hi, c := fr.expr(syntax.NodeID(node.Rhs))
	if c != nil {
		return false
	}
	hiOp := token.Lt
	if fr.t.Toks[node.Tok].Kind == token.DotDotEq {
		hiOp = token.LtEq
	}
	return bool(fr.binaryOp(p, token.GtEq, v, lo).(Bool)) && bool(fr.binaryOp(p, hiOp, v, hi).(Bool))
}

// bind declares a pattern binding: a `&T` binding aliases the payload it
// borrows, any other takes its own copy.
func (fr *frame) bind(ent sem.EntityID, v Value) {
	if fr.in.r.Types.Kind(fr.in.r.Entity(ent).Type) == sem.KRef {
		fr.declare(ent, v)
		return
	}
	fr.declare(ent, cloneValue(v))
}

// patternBinds reports whether a pattern introduces any binding.
func (fr *frame) patternBinds(p syntax.NodeID) bool {
	if p == 0 {
		return false
	}
	if fr.t.Kind(p) == syntax.PatBind {
		return true
	}
	found := false
	fr.t.EachChild(p, func(ch syntax.NodeID) { found = found || fr.patternBinds(ch) })
	return found
}

// matchTuple matches `(a, b)` positionally against a TupleN record.
func (fr *frame) matchTuple(p syntax.NodeID, v Value) bool {
	rec, ok := v.(*Record)
	if !ok {
		return false
	}
	for i, s := range fr.t.Children(p) {
		if i >= len(rec.Fields) || !fr.match(s, rec.Fields[i]) {
			return false
		}
	}
	return true
}

func (fr *frame) matchCtor(p syntax.NodeID, v Value) bool {
	node := fr.t.Nodes[p]
	ent := fr.info.Uses[p]
	subs := fr.t.Children(syntax.NodeID(node.Rhs))
	e := fr.in.r.Entity(ent)
	if e.Kind != sem.EntVariant {
		box, ok := v.(*Box)
		if !ok {
			return false
		}
		dn := fr.in.r.Types.Node(box.Dyn)
		if dn.Kind != sem.KNamed || dn.Ent != ent {
			return false
		}
		if len(subs) == 1 {
			return fr.match(subs[0], box.V)
		}
		return true
	}
	vr, ok := v.(*Variant)
	if !ok || vr.V != ent {
		return false
	}
	for i, s := range subs {
		if i >= len(vr.Payload) || !fr.match(s, vr.Payload[i]) {
			return false
		}
	}
	return true
}

func (fr *frame) matchRecord(p syntax.NodeID, v Value) bool {
	node := fr.t.Nodes[p]
	ent := fr.info.Uses[p]
	fields := fr.t.Children(syntax.NodeID(node.Rhs))
	e := fr.in.r.Entity(ent)
	var get func(name string) (Value, bool)
	switch x := v.(type) {
	case *Record:
		if e.Kind != sem.EntType {
			return false
		}
		get = func(name string) (Value, bool) {
			fld := fr.in.r.FindField(ent, name)
			if fld == 0 {
				return nil, false
			}
			return x.Fields[fr.in.r.Field(fld).Index], true
		}
	case *Variant:
		if x.V != ent {
			return false
		}
		vi := fr.in.r.Variant(ent)
		get = func(name string) (Value, bool) {
			for i, n := range vi.Names {
				if n == name {
					return x.Payload[i], true
				}
			}
			return nil, false
		}
	default:
		return false
	}
	for _, f := range fields {
		fn := fr.t.Nodes[f]
		val, ok := get(fr.t.TokText(fn.Tok))
		if !ok {
			return false
		}
		if fn.Lhs != 0 {
			if !fr.match(syntax.NodeID(fn.Lhs), val) {
				return false
			}
			continue
		}
		fr.bind(fr.info.Defs[f], val)
	}
	return true
}
