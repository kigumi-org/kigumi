package sem

import "kigumi/internal/syntax"

// declareLayout validates `layout(C)` / `layout(C, packed)`.
func (r *Result) declareLayout(f FileID, id EntityID, clause syntax.NodeID) {
	if clause == 0 {
		return
	}
	t := r.tree(f)
	info := r.typeDecl(id)
	if info.Form != FormRecord && info.Form != FormResource {
		r.errAt(f, clause, cLayoutTarget)
		return
	}
	var hasC, hasPacked bool
	for _, a := range t.Children(syntax.NodeID(t.Nodes[clause].Lhs)) {
		if t.Kind(a) == syntax.FieldInit {
			r.declareLayoutAlign(f, info, a)
			continue
		}
		text := ""
		if t.Kind(a) == syntax.Ident {
			text = t.TokText(t.Nodes[a].Tok)
		}
		switch text {
		case "C":
			hasC = true
		case "packed":
			hasPacked = true
		default:
			r.errAt(f, a, cLayoutUnknown, string(t.File.Src[t.Span(a).Start:t.Span(a).End]))
		}
	}
	// packed subsumes C (still C-representable fields, no padding), so it
	// wins regardless of which attribute the source lists first.
	switch {
	case hasPacked:
		info.Layout = "packed"
	case hasC:
		info.Layout = "C"
	}
}

// declareLayoutAlign validates `align: N`: N must be a
// compile-time power of two within the target's max alignment.
func (r *Result) declareLayoutAlign(f FileID, info *TypeDeclInfo, item syntax.NodeID) {
	t := r.tree(f)
	if t.TokText(t.Nodes[item].Tok) != "align" {
		r.errAt(f, item, cLayoutUnknown, t.TokText(t.Nodes[item].Tok))
		return
	}
	expr := syntax.NodeID(t.Nodes[item].Lhs)
	v, ok := r.evalConst(f, expr)
	if !ok {
		return
	}
	maxAlign := r.maxCAlign()
	if v.Kind != constInt || v.Int.Sign() <= 0 || !v.Int.IsInt64() || !isPow2(v.Int.Int64()) || v.Int.Int64() > maxAlign {
		raw := string(t.File.Src[t.Span(expr).Start:t.Span(expr).End])
		r.errAt(f, expr, cLayoutAlign, maxAlign, raw)
		return
	}
	info.Align = v.Int.Int64()
}

func isPow2(n int64) bool { return n > 0 && n&(n-1) == 0 }

// maxCAlign is the strictest alignment an ABI-safe type can need: 8 bytes
// for fixed-width integers and f64, or the pointer width if wider.
func (r *Result) maxCAlign() int64 {
	return int64(max(8, r.Types.PtrBits()/8))
}

// checkLayoutFields keeps a layout(C) record to C-representable fields:
// fixed-width numbers, raw pointers and nested layout(C) records.
func (r *Result) checkLayoutFields(id EntityID) {
	info := r.typeDecl(id)
	if info.Layout == "" {
		return
	}
	e := &r.Entities[id]
	if len(info.Params) > 0 {
		r.errAt(e.File, e.Node, cLayoutGeneric)
	}
	for _, fld := range info.Fields {
		t := r.Entities[fld].Type
		if !r.cRepresentable(t) {
			r.errAt(e.File, r.Entities[fld].Node, cLayoutFieldType, r.Entities[fld].Name, r.TypeString(t))
		}
	}
}

func (r *Result) cRepresentable(t TypeID) bool {
	tt := r.Types
	if t == TyPoison || tt.IsNumeric(t) || tt.Kind(t) == KPtr || tt.IsCFn(t) || r.cRecord(t) {
		return true
	}
	elem, _, ok := r.fixedArrayShape(t)
	return ok && tt.IsNumeric(elem)
}

// fixedArrayShape recognizes FixedArray[elem, N] with N a literal length:
// the only const-generic instance a layout(C) field
// or an extern(C) signature can name, since neither carries a hidden
// length argument.
func (r *Result) fixedArrayShape(t TypeID) (elem TypeID, n int64, ok bool) {
	tt := r.Types
	if tt.Kind(t) != KNamed {
		return 0, 0, false
	}
	node := tt.Node(t)
	if node.Ent == 0 || node.Ent != r.langItem(0, 0, "FixedArray") || len(node.Args) != 2 || !tt.IsConst(node.Args[1]) {
		return 0, 0, false
	}
	v, _ := tt.ConstValue(node.Args[1])
	return node.Args[0], v, true
}
