package interp

import (
	"kigumi/internal/sem"
)

// Value is a runtime value. Structural values are copied by cloneValue at
// binding, argument and field boundaries; containers are shared only
// through cells.
type Value any

type Int struct {
	V int64
	T sem.TypeID
}

type Float struct {
	V float64
	T sem.TypeID
}

type Bool bool

type Str string

type Bytes []byte

type Char rune

type Unit struct{}

type Record struct {
	Type   sem.TypeID
	Fields []Value
}

type Variant struct {
	Type    sem.TypeID
	V       sem.EntityID
	Payload []Value
}

type Array struct {
	Elems []Value
}

type FnItem struct {
	Fn sem.EntityID
}

type Closure struct {
	Ent sem.EntityID
	Env map[sem.EntityID]*Cell
}

// Partial is a function with its trailing witness parameters bound.
type Partial struct {
	Fn    sem.EntityID
	Bound []Value
}

// Box is an existential: a value with its dynamic type.
type Box struct {
	Dyn sem.TypeID
	V   Value
}

// Ref is a borrow of a place.
type Ref struct {
	Cell *Cell
	Path []int
}

// Opaque holds host objects (capabilities, handles, plans).
type Opaque struct {
	Kind string
	Data any
}

// Moved marks a cell whose value was moved out.
type Moved struct{}

// Cell is the storage of one binding.
type Cell struct {
	V Value
}

// cloneValue copies a structural value; identity handles and closures
// share their targets.
func cloneValue(v Value) Value {
	switch x := v.(type) {
	case *Record:
		out := &Record{Type: x.Type, Fields: make([]Value, len(x.Fields))}
		for i, f := range x.Fields {
			out.Fields[i] = cloneValue(f)
		}
		return out
	case *Variant:
		out := &Variant{Type: x.Type, V: x.V, Payload: make([]Value, len(x.Payload))}
		for i, p := range x.Payload {
			out.Payload[i] = cloneValue(p)
		}
		return out
	case *Array:
		out := &Array{Elems: make([]Value, len(x.Elems))}
		for i, e := range x.Elems {
			out.Elems[i] = cloneValue(e)
		}
		return out
	case *Box:
		return &Box{Dyn: x.Dyn, V: cloneValue(x.V)}
	}
	return v
}

// valueEqual is structural equality (`==`).
func valueEqual(a, b Value) bool {
	switch x := a.(type) {
	case Int:
		y, ok := b.(Int)
		return ok && x.V == y.V
	case Float:
		y, ok := b.(Float)
		return ok && x.V == y.V
	case Bool:
		y, ok := b.(Bool)
		return ok && x == y
	case Str:
		y, ok := b.(Str)
		return ok && x == y
	case Bytes:
		y, ok := b.(Bytes)
		return ok && string(x) == string(y)
	case Char:
		y, ok := b.(Char)
		return ok && x == y
	case Unit:
		_, ok := b.(Unit)
		return ok
	case *Record:
		y, ok := b.(*Record)
		if !ok || len(x.Fields) != len(y.Fields) {
			return false
		}
		for i := range x.Fields {
			if !valueEqual(x.Fields[i], y.Fields[i]) {
				return false
			}
		}
		return true
	case *Variant:
		y, ok := b.(*Variant)
		if !ok || x.V != y.V || len(x.Payload) != len(y.Payload) {
			return false
		}
		for i := range x.Payload {
			if !valueEqual(x.Payload[i], y.Payload[i]) {
				return false
			}
		}
		return true
	case *Array:
		y, ok := b.(*Array)
		if !ok || len(x.Elems) != len(y.Elems) {
			return false
		}
		for i := range x.Elems {
			if !valueEqual(x.Elems[i], y.Elems[i]) {
				return false
			}
		}
		return true
	case *Box:
		y, ok := b.(*Box)
		return ok && x.Dyn == y.Dyn && valueEqual(x.V, y.V)
	case *Opaque:
		y, ok := b.(*Opaque)
		return ok && x == y
	}
	return false
}
