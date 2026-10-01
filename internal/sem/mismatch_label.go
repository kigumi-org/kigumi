package sem

import "strings"

// mismatchLabels qualifies want/got with their declaring package when they
// share a bare name, so the message doesn't read "expected `Widget`, found
// `Widget`" with no way to tell them apart.
func (r *Result) mismatchLabels(want, got TypeID) (string, string) {
	we, ge := r.namedTypeEntity(want), r.namedTypeEntity(got)
	if we != 0 && ge != 0 && we != ge {
		wf, gf := &r.Entities[we], &r.Entities[ge]
		if wf.Name == gf.Name && wf.Pkg != gf.Pkg {
			return r.qualifiedTypeString(want, we), r.qualifiedTypeString(got, ge)
		}
	}
	// Two lifetime parameters from different declarations can both be
	// spelled `'a` and render identically; qualify each
	// with its declaring function or type.
	if qualify := r.collidingLifetimeParams(want, got); qualify != nil {
		var wsb, gsb strings.Builder
		r.writeTypeQualifyingLifetimes(&wsb, want, qualify)
		r.writeTypeQualifyingLifetimes(&gsb, got, qualify)
		return wsb.String(), gsb.String()
	}
	return r.TypeString(want), r.TypeString(got)
}

func (r *Result) collidingLifetimeParams(a, b TypeID) map[EntityID]bool {
	names := map[string]EntityID{}
	for _, p := range r.Types.Params(a) {
		if r.typeParam(p).IsLifetime {
			names[r.Entities[p].Name] = p
		}
	}
	var out map[EntityID]bool
	for _, p := range r.Types.Params(b) {
		if !r.typeParam(p).IsLifetime {
			continue
		}
		if other, ok := names[r.Entities[p].Name]; ok && other != p {
			if out == nil {
				out = map[EntityID]bool{}
			}
			out[p], out[other] = true, true
		}
	}
	return out
}

// Only KNamed/KIface arguments are walked specially: a lifetime tag never
// occurs elsewhere in a TypeID.
func (r *Result) writeTypeQualifyingLifetimes(sb *strings.Builder, t TypeID, qualify map[EntityID]bool) {
	tt := r.Types
	n := tt.nodes[t]
	if n.Kind == KParam && qualify[n.Ent] {
		sb.WriteString(r.entityName(n.Ent))
		sb.WriteString(" (")
		sb.WriteString(r.entityName(r.Entities[n.Ent].Parent))
		sb.WriteString(")")
		return
	}
	if n.Kind == KNamed || n.Kind == KIface {
		if _, ok := tt.IsOption(t); !ok {
			if _, _, ok := tt.IsResult(t); !ok && len(n.Args) > 0 {
				sb.WriteString(r.entityName(n.Ent))
				sb.WriteString("[")
				for i, a := range n.Args {
					if i > 0 {
						sb.WriteString(", ")
					}
					r.writeTypeQualifyingLifetimes(sb, a, qualify)
				}
				sb.WriteString("]")
				return
			}
		}
	}
	r.writeType(sb, t)
}

func (r *Result) namedTypeEntity(t TypeID) EntityID {
	if t == 0 {
		return 0
	}
	tt := r.Types
	n := tt.nodes[t]
	if n.Kind != KNamed && n.Kind != KIface {
		return 0
	}
	if _, ok := tt.IsOption(t); ok {
		return 0
	}
	if _, _, ok := tt.IsResult(t); ok {
		return 0
	}
	return n.Ent
}

func (r *Result) qualifiedTypeString(t TypeID, ent EntityID) string {
	var sb strings.Builder
	if pkg := r.Entities[ent].Pkg; pkg != 0 && int(pkg) < len(r.Packages) {
		if path := r.Packages[pkg].Path; path != "" {
			sb.WriteString(path)
			sb.WriteString(".")
		}
	}
	sb.WriteString(r.entityName(ent))
	r.writeArgs(&sb, r.Types.nodes[t].Args)
	return sb.String()
}
