package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// member evaluates `a.b`: a field, a method value or a package/type member.
func (fr *frame) member(n syntax.NodeID) (Value, *ctrl) {
	node := fr.t.Nodes[n]
	ent := fr.info.Uses[n]
	in := fr.in
	if call, ok := fr.info.Calls[n]; ok && call.Kind == sem.CallMethodValue {
		recv, c := fr.expr(syntax.NodeID(node.Lhs))
		if c != nil {
			return nil, c
		}
		return &BoundMethod{Fn: call.Callee, Recv: cloneValue(deref(recv))}, nil
	}
	e := in.r.Entity(ent)
	switch e.Kind {
	case sem.EntField:
		base, c := fr.expr(syntax.NodeID(node.Lhs))
		if c != nil {
			return nil, c
		}
		rec, ok := deref(base).(*Record)
		if !ok {
			fr.panicAt(n, "field access on a non-record (%T)", deref(base))
		}
		return rec.Fields[in.r.Field(ent).Index], nil
	case sem.EntFn:
		return &FnItem{Fn: ent}, nil
	case sem.EntConst:
		return in.constValue(ent, fr.typeOf(n)), nil
	case sem.EntVariant:
		return &Variant{Type: fr.typeOf(n), V: ent}, nil
	}
	fr.panicAt(n, "cannot evaluate member `%s`", fr.t.TokText(node.Tok))
	return nil, nil
}

func (fr *frame) optMember(n syntax.NodeID) (Value, *ctrl) {
	node := fr.t.Nodes[n]
	base, c := fr.expr(syntax.NodeID(node.Lhs))
	if c != nil {
		return nil, c
	}
	vr := deref(base).(*Variant)
	if len(vr.Payload) == 0 {
		return &Variant{Type: fr.typeOf(n), V: vr.V}, nil
	}
	rec := deref(vr.Payload[0]).(*Record)
	fld := fr.info.Uses[n]
	inner := rec.Fields[fr.in.r.Field(fld).Index]
	return &Variant{Type: fr.typeOf(n), V: vr.V, Payload: []Value{inner}}, nil
}

// BoundMethod is a method value: the receiver captured by copy.
type BoundMethod struct {
	Fn   sem.EntityID
	Recv Value
}

func (fr *frame) try(n syntax.NodeID) (Value, *ctrl) {
	node := fr.t.Nodes[n]
	v, c := fr.expr(syntax.NodeID(node.Lhs))
	if c != nil {
		return nil, c
	}
	vr := deref(v).(*Variant)
	if fr.in.r.Entity(vr.V).Name == "Ok" {
		return vr.Payload[0], nil
	}
	e := vr.Payload[0]
	if call, ok := fr.info.Calls[n]; ok && len(call.Inst) == 1 {
		if _, already := e.(*Box); !already {
			e = &Box{Dyn: call.Inst[0], V: e}
		}
	}
	return nil, &ctrl{kind: ctrlFail, val: e}
}

func (fr *frame) recordLit(n syntax.NodeID) (Value, *ctrl) {
	node := fr.t.Nodes[n]
	call := fr.info.Calls[n]
	info := fr.in.r.TypeDecl(call.Callee)
	rec := &Record{Type: fr.typeOf(n), Fields: make([]Value, len(info.Fields))}
	for i := range rec.Fields {
		rec.Fields[i] = Unit{}
	}
	for _, entry := range fr.t.Children(syntax.NodeID(node.Rhs)) {
		en := fr.t.Nodes[entry]
		v, c := fr.expr(syntax.NodeID(en.Lhs))
		if c != nil {
			return nil, c
		}
		if en.Kind == syntax.Spread {
			src := deref(v).(*Record)
			srcInfo := fr.in.r.TypeDecl(fr.in.r.Types.Node(src.Type).Ent)
			for i, fld := range info.Fields {
				name := fr.in.r.Entity(fld).Name
				if sf := fr.in.r.FindField(fr.in.r.Types.Node(src.Type).Ent, name); sf != 0 {
					rec.Fields[i] = cloneValue(src.Fields[srcIndex(srcInfo, sf)])
				}
			}
			continue
		}
		fld := fr.info.Uses[entry]
		rec.Fields[fr.in.r.Field(fld).Index] = fr.transfer(syntax.NodeID(en.Lhs), v)
	}
	return rec, nil
}

// tupleLit evaluates a tuple literal: each element sets the field at its
// position, in order, of the predeclared TupleN record the checker resolved.
func (fr *frame) tupleLit(n syntax.NodeID) (Value, *ctrl) {
	items := fr.t.Children(n)
	rec := &Record{Type: fr.typeOf(n), Fields: make([]Value, len(items))}
	for i, it := range items {
		v, c := fr.expr(it)
		if c != nil {
			return nil, c
		}
		rec.Fields[i] = fr.transfer(it, v)
	}
	return rec, nil
}

func srcIndex(info *sem.TypeDeclInfo, fld sem.EntityID) int {
	for i, f := range info.Fields {
		if f == fld {
			return i
		}
	}
	return 0
}

func (fr *frame) lambda(n syntax.NodeID) Value {
	ent := fr.info.Defs[n]
	info := fr.in.r.Closure(ent)
	env := map[sem.EntityID]*Cell{}
	for _, cap := range info.Captures {
		c := fr.cell(cap.Local)
		if cap.Mode == sem.CapCell {
			env[cap.Local] = c
		} else {
			env[cap.Local] = &Cell{V: cloneValue(c.V)}
		}
	}
	return &Closure{Ent: ent, Env: env}
}
