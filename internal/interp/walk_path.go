package interp

// walkPath reads the value at path inside v: a Record/Variant/Array
// container indexed by field or element, following any Ref found along the
// way.
func walkPath(v Value, path []int) Value {
	for _, i := range path {
		v = deref(v)
		switch x := v.(type) {
		case *Record:
			v = x.Fields[i]
		case *Array:
			v = x.Elems[i]
		case *Variant:
			v = x.Payload[i]
		default:
			return v
		}
	}
	return deref(v)
}

func deref(v Value) Value {
	for {
		ref, ok := v.(*Ref)
		if !ok {
			return v
		}
		v = walkPath(ref.Cell.V, ref.Path)
	}
}

// setPath writes nv at path inside v, the write counterpart of walkPath.
func setPath(v Value, path []int, nv Value) {
	for k, i := range path {
		v = deref(v)
		last := k == len(path)-1
		switch x := v.(type) {
		case *Record:
			if last {
				x.Fields[i] = nv
				return
			}
			v = x.Fields[i]
		case *Array:
			if last {
				x.Elems[i] = nv
				return
			}
			v = x.Elems[i]
		case *Variant:
			if last {
				x.Payload[i] = nv
				return
			}
			v = x.Payload[i]
		}
	}
}
