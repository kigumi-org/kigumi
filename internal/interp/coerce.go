package interp

import (
	"math/big"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// coerce applies the checker's recorded coercion steps to a value.
func (fr *frame) coerce(n syntax.NodeID, v Value) Value {
	co, ok := fr.info.Coerce[n]
	if !ok {
		return v
	}
	in := fr.in
	for _, s := range co.Steps {
		switch s.Kind {
		case sem.CoLiteral:
			v = retype(v, s.To, in)
		case sem.CoBorrow:
			v = &Ref{Cell: &Cell{V: v}}
		case sem.CoSome:
			v = &Variant{Type: s.To, V: in.variantNamed(in.r.Types.OptionEnt(), "Some"), Payload: []Value{v}}
		case sem.CoOk:
			v = &Variant{Type: s.To, V: in.variantNamed(in.r.Types.ResultEnt(), "Ok"), Payload: []Value{v}}
		case sem.CoExistential:
			if _, already := v.(*Box); !already {
				v = &Box{Dyn: co.From, V: v}
			}
		}
	}
	return v
}

func retype(v Value, to sem.TypeID, in *Interp) Value {
	switch x := v.(type) {
	case Int:
		if in.r.Types.IsFloat(to) {
			return mkFloat(in, float64(x.V), to)
		}
		return Int{V: x.V, T: to}
	case Float:
		return mkFloat(in, x.V, to)
	}
	return v
}

func (in *Interp) variantNamed(adt sem.EntityID, name string) sem.EntityID {
	for _, v := range in.r.TypeDecl(adt).Variants {
		if in.r.Entity(v).Name == name {
			return v
		}
	}
	return 0
}

// literal builds the value of a numeric literal node from the folded
// constant and its adopted type.
func (fr *frame) literal(n syntax.NodeID) Value {
	lit, ok := fr.in.r.LiteralOf(fr.t, n)
	if !ok {
		fr.panicAt(n, "literal without a value")
	}
	t := fr.typeOf(n)
	tt := fr.in.r.Types
	if tt.Kind(t) == sem.KUntyped || t == 0 {
		if lit.Kind == sem.LitFloat {
			t = sem.TyF64
		} else {
			t = sem.TyI64
		}
	}
	if tt.IsFloat(t) {
		if lit.Kind == sem.LitInt {
			f, _ := new(big.Float).SetInt(lit.Int).Float64()
			return mkFloat(fr.in, f, t)
		}
		f, _ := lit.Float.Float64()
		return mkFloat(fr.in, f, t)
	}
	if lit.Kind == sem.LitInt {
		if lit.Int.IsInt64() {
			return Int{V: lit.Int.Int64(), T: t}
		}
		return Int{V: int64(lit.Int.Uint64()), T: t}
	}
	f, _ := lit.Float.Float64()
	return Float{V: f, T: t}
}
