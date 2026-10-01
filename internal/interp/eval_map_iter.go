package interp

import "kigumi/internal/sem"

// isMapRecord reports whether x is a std/map Map[K, V] value: its `for`
// element is a (K, V) tuple built from keyList/valueList,
// not the record's own fields in order.
func (fr *frame) isMapRecord(x *Record) bool {
	me := fr.in.r.MapEntity()
	return me != 0 && fr.in.r.Types.Node(x.Type).Ent == me
}

// mapEntries builds x's `for` elements: a (K, V) tuple per entry, in
// keyList/valueList's shared insertion order.
func (fr *frame) mapEntries(x *Record) []Value {
	keys, vals := x.Fields[0].(*Array), x.Fields[1].(*Array)
	tupType := fr.tupleType(x.Type)
	out := make([]Value, len(keys.Elems))
	for i := range keys.Elems {
		out[i] = &Record{Type: tupType, Fields: []Value{keys.Elems[i], vals.Elems[i]}}
	}
	return out
}

// tupleType is the (K, V) tuple type for a Map[K, V] value of type mapType.
func (fr *frame) tupleType(mapType sem.TypeID) sem.TypeID {
	args := fr.in.r.Types.Node(mapType).Args
	tup, _ := fr.in.r.Types.TupleEntity(2)
	return fr.in.r.Types.Named(tup, []sem.TypeID{args[0], args[1]})
}
