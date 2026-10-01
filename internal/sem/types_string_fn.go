package sem

import "strings"

func (r *Result) writeFn(sb *strings.Builder, n typeNode) {
	if n.Flags&fnCAbi != 0 {
		sb.WriteString("extern(C) ")
	}
	eff := Effects(n.Flags)
	if n.Flags&fnEffectPoly != 0 {
		sb.WriteString("pure? ")
		eff &^= EffPure
	}
	for _, e := range []struct {
		bit  Effects
		name string
	}{{EffPure, "pure "}, {EffNoalloc, "noalloc "}, {EffAsync, "async "}, {EffUnsafe, "unsafe "}} {
		if eff&e.bit != 0 {
			sb.WriteString(e.name)
		}
	}
	sb.WriteString("fn(")
	for i, a := range n.Args {
		if i > 0 {
			sb.WriteString(", ")
		}
		if i == len(n.Args)-1 && n.Flags&fnVariadic != 0 {
			sb.WriteString("...")
			if elem, ok := r.arrayElem(a); ok {
				a = elem
			}
		}
		r.writeType(sb, a)
	}
	if n.Flags&fnCVariadic != 0 {
		if len(n.Args) > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("...")
	}
	sb.WriteString(") -> ")
	r.writeType(sb, n.Elem)
}

func (r *Result) arrayElem(t TypeID) (TypeID, bool) {
	n := r.Types.nodes[t]
	if n.Kind == KNamed && n.Ent == r.Types.arrayEnt && r.Types.arrayEnt != 0 && len(n.Args) == 1 {
		return n.Args[0], true
	}
	return 0, false
}
