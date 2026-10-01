package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// resolvePlace turns an lvalue expression into a cell and a path of field
// or element indices; a Ref stored in a cell is followed.
func (fr *frame) resolvePlace(n syntax.NodeID) (*Cell, []int, *ctrl) {
	node := fr.t.Nodes[n]
	switch node.Kind {
	case syntax.Paren:
		return fr.resolvePlace(syntax.NodeID(node.Lhs))
	case syntax.Ident:
		ent := fr.info.Uses[n]
		if k := fr.in.r.Entity(ent).Kind; k != sem.EntLocal && k != sem.EntParam {
			break
		}
		c := fr.cell(ent)
		if ref, ok := c.V.(*Ref); ok {
			return ref.Cell, append([]int{}, ref.Path...), nil
		}
		return c, nil, nil
	case syntax.MemberExpr:
		fld := fr.info.Uses[n]
		if fr.in.r.Entity(fld).Kind != sem.EntField {
			break
		}
		cell, path, c := fr.resolvePlace(syntax.NodeID(node.Lhs))
		if c != nil {
			return nil, nil, c
		}
		return cell, append(path, fr.in.r.Field(fld).Index), nil
	case syntax.BracketExpr:
		cell, path, c := fr.resolvePlace(syntax.NodeID(node.Lhs))
		if c != nil {
			return nil, nil, c
		}
		items := fr.t.Children(syntax.NodeID(node.Rhs))
		iv, c := fr.expr(items[0])
		if c != nil {
			return nil, nil, c
		}
		i, ok := iv.(Int)
		if !ok {
			fr.panicAt(n, "index is not an integer")
		}
		cur := walkPath(cell.V, path)
		arr, isArr := cur.(*Array)
		if !isArr || i.V < 0 || int(i.V) >= len(arr.Elems) {
			fr.panicAt(n, "index %d out of range", i.V)
		}
		return cell, append(path, int(i.V)), nil
	}
	tmp := &Cell{}
	v, c := fr.expr(n)
	if c != nil {
		return nil, nil, c
	}
	tmp.V = v
	return tmp, nil, nil
}

func (fr *frame) setPlace(n syntax.NodeID, v Value) *ctrl {
	cell, path, c := fr.resolvePlace(n)
	if c != nil {
		return c
	}
	fr.writePlace(cell, path, v)
	return nil
}

// writePlace stores v at an already-resolved place, dropping what it
// overwrites. Split out of setPlace so a compound assignment can
// resolve the place once and both read and write it, instead of
// re-evaluating the base/index expression for the write.
func (fr *frame) writePlace(cell *Cell, path []int, v Value) {
	if len(path) == 0 {
		if ref, ok := cell.V.(*Ref); ok {
			fr.dropOld(walkPath(ref.Cell.V, ref.Path))
			setPath(ref.Cell.V, ref.Path, v)
			return
		}
		fr.dropOld(cell.V)
		cell.V = v
		return
	}
	fr.dropOld(walkPath(cell.V, path))
	setPath(cell.V, path, v)
}

// dropOld destroys the value an assignment overwrites.
func (fr *frame) dropOld(old Value) {
	if _, moved := old.(Moved); moved || old == nil {
		return
	}
	if fr.in.moveOnly(old) {
		fr.dropDeep(old)
	}
}

// dropDeep runs the destructors inside a value: the resource's own drop
// first, then its owning fields in reverse declaration order.
func (fr *frame) dropDeep(v Value) {
	switch x := deref(v).(type) {
	case *Record:
		fr.dropValue(x)
		for i := len(x.Fields) - 1; i >= 0; i-- {
			if fr.in.moveOnly(x.Fields[i]) {
				fr.dropDeep(x.Fields[i])
			}
		}
	case *Variant:
		for _, p := range x.Payload {
			if fr.in.moveOnly(p) {
				fr.dropDeep(p)
			}
		}
	case *Array:
		for i := len(x.Elems) - 1; i >= 0; i-- {
			if fr.in.moveOnly(x.Elems[i]) {
				fr.dropDeep(x.Elems[i])
			}
		}
	case *Future:
		for i := len(x.Args) - 1; i >= 0; i-- {
			if fr.in.moveOnly(x.Args[i]) {
				fr.dropDeep(x.Args[i])
			}
		}
	case *Opaque:
		switch x.Kind {
		case "Shared":
			fr.dropShared(x.Data.(*sharedBox))
		case "Task":
			if f, ok := x.Data.(Value); ok && fr.in.moveOnly(f) {
				fr.dropDeep(f)
			}
		}
	case *Box:
		if fr.in.moveOnly(x.V) {
			fr.dropDeep(x.V)
		}
	}
}

// borrow makes a Ref to a place, or wraps a temporary.
func (fr *frame) borrow(n syntax.NodeID) (Value, *ctrl) {
	cell, path, c := fr.resolvePlace(n)
	if c != nil {
		return nil, c
	}
	return &Ref{Cell: cell, Path: path}, nil
}
