package interp

import "kigumi/internal/sem"

// moveOnly reports whether a runtime value owns a resource.
func (in *Interp) moveOnly(v Value) bool {
	switch x := v.(type) {
	case *Record:
		node := in.r.Types.Node(x.Type)
		if node.Kind == sem.KNamed && in.r.TypeDecl(node.Ent).Form == sem.FormResource {
			return true
		}
		for _, f := range x.Fields {
			if in.moveOnly(f) {
				return true
			}
		}
	case *Variant:
		for _, p := range x.Payload {
			if in.moveOnly(p) {
				return true
			}
		}
	case *Array:
		for _, e := range x.Elems {
			if in.moveOnly(e) {
				return true
			}
		}
	case *Future:
		for _, a := range x.Args {
			if in.moveOnly(a) {
				return true
			}
		}
	case *Opaque:
		switch x.Kind {
		case "Shared":
			return true
		case "Task":
			if f, ok := x.Data.(Value); ok {
				return in.moveOnly(f)
			}
		}
	case *Box:
		return in.moveOnly(x.V)
	}
	return false
}

// dropValue runs the destructor of a resource value.
func (fr *frame) dropValue(v Value) {
	rec, ok := v.(*Record)
	if !ok {
		return
	}
	in := fr.in
	node := in.r.Types.Node(rec.Type)
	if node.Kind != sem.KNamed {
		return
	}
	info := in.r.TypeDecl(node.Ent)
	if info.Form == sem.FormResource && info.Drop != 0 {
		in.callFnValue(fr, info.Drop, 0, nil, rec)
	}
}
