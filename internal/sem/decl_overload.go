package sem

// checkOverloadSets rejects two members with the same parameter types
// and members of one set with different visibility.
func (r *Result) checkOverloadSets() {
	for set := 1; set < len(r.Overloads); set++ {
		members := r.Overloads[set].Members
		for i := 1; i < len(members); i++ {
			m := members[i]
			e := &r.Entities[m]
			if e.Flags&EfPoison != 0 {
				continue
			}
			for _, prev := range members[:i] {
				if r.Entities[prev].Flags&EfPoison != 0 {
					continue
				}
				if r.sameParams(prev, m) {
					r.errAt(e.File, e.Node, cFnRedeclared, e.Name, r.paramTypesText(m))
					e.Flags |= EfPoison
					break
				}
				if r.Entities[prev].Vis != e.Vis {
					r.errAt(e.File, e.Node, cOverloadVisibility, e.Name)
					break
				}
			}
		}
	}
}

func (r *Result) sameParams(a, b EntityID) bool {
	fa, fb := r.Fn(a), r.Fn(b)
	na, nb := r.Types.Node(fa.Sig), r.Types.Node(fb.Sig)
	if fa.Recv != fb.Recv || len(na.Args) != len(nb.Args) || na.Flags&fnVariadic != nb.Flags&fnVariadic {
		return false
	}
	for i := range na.Args {
		if na.Args[i] != nb.Args[i] {
			return false
		}
	}
	return true
}

func (r *Result) paramTypesText(id EntityID) string {
	out := ""
	for i, p := range r.Types.Node(r.Fn(id).Sig).Args {
		if i > 0 {
			out += ", "
		}
		out += r.TypeString(p)
	}
	return out
}
