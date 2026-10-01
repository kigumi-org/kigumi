package sem

import "kigumi/internal/syntax"

// metaAttrs resolves the dotted metadata attributes of a declaration or
// parameter (e.g. `@web.get("/x")`).
func (r *Result) metaAttrs(f FileID, list syntax.NodeID, onType bool) []MetaRef {
	if list == 0 {
		return nil
	}
	t := r.tree(f)
	var dotted []syntax.NodeID
	for _, a := range t.Children(list) {
		path := syntax.NodeID(t.Nodes[a].Lhs)
		switch name := pathText(t, path, "."); {
		case len(t.PathToks(path)) > 1:
			dotted = append(dotted, a)
		case name == "impl":
			if !onType {
				r.errAt(f, a, cAttrWrongTarget)
			}
		default:
			r.errAt(f, a, cAttrUnknown, name)
		}
	}
	if len(dotted) == 0 {
		return nil
	}
	return r.resolveMetadata(f, 0, t.AddList(syntax.Metadata, t.Nodes[dotted[0]].Tok, dotted))
}

func (r *Result) rejectAttrs(f FileID, list syntax.NodeID) {
	if list == 0 {
		return
	}
	t := r.tree(f)
	for _, a := range t.Children(list) {
		name := pathText(t, syntax.NodeID(t.Nodes[a].Lhs), ".")
		if name == "impl" {
			r.errAt(f, a, cAttrWrongTarget)
		} else {
			r.errAt(f, a, cAttrUnknown, name)
		}
	}
}

// resolveMetadata resolves `json.name("id")` heads by ordinary name lookup;
// schema validation belongs to library comptime.
func (r *Result) resolveMetadata(f FileID, fld, meta syntax.NodeID) []MetaRef {
	if meta == 0 {
		return nil
	}
	t := r.tree(f)
	var out []MetaRef
	seen := map[string]bool{}
	for _, item := range t.Children(meta) {
		path := syntax.NodeID(t.Nodes[item].Lhs)
		head := r.resolveValueName(f, r.fileScopes[f], path)
		// No metadata item is repeatable today,
		// so a repeated name is always a duplicate, even with different
		// arguments that would otherwise silently override each other.
		key := pathText(t, path, ".")
		if seen[key] {
			r.errAt(f, item, cMetadataDuplicate, key)
		}
		seen[key] = true
		if head != 0 {
			r.Files[f].Uses[item] = head
			// Field metadata is resolved before signatures exist; checkSchemas
			// validates those heads later.
			if r.Entities[head].Kind == EntFn && r.Fn(head).Sig != 0 && !r.isSchema(head) {
				r.errAt(f, path, cAttrNotSchema, pathText(t, path, "."))
				continue
			}
		}
		ref := MetaRef{Head: head, Node: item}
		for _, a := range t.Children(syntax.NodeID(t.Nodes[item].Rhs)) {
			if v, ok := r.evalConst(f, a); ok {
				lit, _ := v.literal()
				ref.Args = append(ref.Args, lit)
			}
		}
		out = append(out, ref)
	}
	return out
}

// checkSchemas validates the heads of field metadata once signatures are
// known.
func (r *Result) checkSchemas(f FileID, refs []MetaRef) {
	t := r.tree(f)
	for _, m := range refs {
		if m.Head != 0 && !r.isSchema(m.Head) {
			path := syntax.NodeID(t.Nodes[m.Node].Lhs)
			r.errAt(f, path, cAttrNotSchema, pathText(t, path, "."))
		}
	}
}

// isSchema accepts a function whose result type is named Metadata (e.g.
// std/json's `name`).
func (r *Result) isSchema(head EntityID) bool {
	e := &r.Entities[head]
	if e.Kind != EntFn {
		return false
	}
	ret := r.Types.Node(r.Fn(head).Sig).Elem
	n := r.Types.Node(ret)
	return n.Kind == KNamed && r.Entities[n.Ent].Name == "Metadata"
}
