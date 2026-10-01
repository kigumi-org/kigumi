package sem

import "sort"

// Member is one name a value or a type offers after a dot: a field, a
// method, an associated function, a variant, or an interface requirement.
type Member struct {
	Name string
	Ent  EntityID
}

// Members lists what `.` can reach on a value of type t, or on the type
// itself when static is set (associated functions and variants); editors
// use it for completion.
func (r *Result) Members(t TypeID, static bool) []Member {
	n := r.Types.Node(t)
	var out []Member
	add := func(name string, ent EntityID) { out = append(out, Member{Name: name, Ent: ent}) }
	switch n.Kind {
	case KRef, KPtr:
		return r.Members(n.Elem, static)
	case KParam, KIface:
		var ifaces []TypeID
		if n.Kind == KIface {
			ifaces = append(ifaces, t)
		} else {
			for _, con := range r.typeParam(n.Ent).Constraints {
				if con.Kind == CIface {
					ifaces = append(ifaces, con.Type)
				}
			}
		}
		for _, it := range ifaces {
			for _, req := range r.iface(r.Types.Node(it).Ent).Reqs {
				if (r.Fn(req).Recv == RecvNone) == static {
					add(r.Entities[req].Name, req)
				}
			}
		}
	case KNamed, KPrim:
		owner := n.Ent
		if n.Kind == KPrim {
			owner = r.primEntity[t]
		}
		if owner == 0 || r.Entities[owner].Kind != EntType {
			break
		}
		info := r.typeDecl(owner)
		for name, set := range info.Members {
			for _, m := range r.Overloads[set].Members {
				if (r.Fn(m).Recv == RecvNone) == static {
					add(name, m)
					break
				}
			}
		}
		if static {
			for _, v := range info.Variants {
				add(r.Entities[v].Name, v)
			}
		} else {
			for _, f := range info.Fields {
				add(r.Entities[f].Name, f)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
