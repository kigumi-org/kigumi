package interp

import (
	"strings"

	"kigumi/internal/hostshell"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// stdFn finds a std function by package path and name.
func (in *Interp) stdFn(pkg, name string) sem.EntityID {
	return in.r.PackageMember(in.r.PackageByPath(pkg), name)
}

// shellLit builds a std/shell.Plan from `$"..."` through the std builders,
// exactly as the native backend does: text pieces and displayed
// interpolations, word and pipe boundaries, redirects.
func (fr *frame) shellLit(n syntax.NodeID) (Value, *ctrl) {
	in := fr.in
	node := fr.t.Nodes[n]
	itemType := in.r.Types.Named(in.stdFn("std/shell", "Item"), nil)
	item := func(name string, payload ...Value) Value {
		return &Variant{Type: itemType, V: in.variantNamed(in.stdFn("std/shell", "Item"), name), Payload: payload}
	}
	p, c := in.callFnValue(fr, in.stdFn("std/shell", "planNew"), n, nil, nil)
	if c != nil {
		return nil, c
	}
	push := func(it Value) *ctrl {
		p, c = in.callFnValue(fr, in.stdFn("std/shell", "planPush"), n, []Value{p, it}, nil)
		return c
	}
	for i, cmd := range fr.t.Children(syntax.NodeID(node.Rhs)) {
		if i > 0 {
			if c := push(item("Pipe")); c != nil {
				return nil, c
			}
		}
		for _, part := range fr.t.Children(cmd) {
			word := part
			if fr.t.Kind(part) == syntax.ShellRedirect {
				op, fd := int64(fr.t.Slot(part, "op")), int64(fr.t.Slot(part, "fd"))
				if c := push(item("Redirect", Int{V: op, T: sem.TyI64}, Int{V: fd, T: sem.TyI64})); c != nil {
					return nil, c
				}
				word = syntax.NodeID(fr.t.Slot(part, "target"))
			} else if c := push(item("Word")); c != nil {
				return nil, c
			}
			text, c := fr.shellWord(word)
			if c != nil {
				return nil, c
			}
			if c := push(item("Text", Str(text))); c != nil {
				return nil, c
			}
		}
	}
	return p, nil
}

func (fr *frame) shellWord(w syntax.NodeID) (string, *ctrl) {
	if w == 0 {
		return "", nil
	}
	var sb strings.Builder
	for _, piece := range fr.t.Children(w) {
		pn := fr.t.Nodes[piece]
		if pn.Kind == syntax.ShellText {
			sb.WriteString(syntax.DecodeString(string(fr.t.File.Src[pn.Lhs:pn.Rhs])))
			continue
		}
		v, c := fr.expr(syntax.NodeID(pn.Lhs))
		if c != nil {
			return "", c
		}
		sb.WriteString(display(v, fr.in))
	}
	return sb.String(), nil
}

// planOf turns a Plan record into the Go plan by asking std/shell.stages
// for the stage records, so word joining lives in one place.
func (fr *frame) planOf(n syntax.NodeID, v Value) (*hostshell.Plan, *ctrl) {
	in := fr.in
	stages, c := in.callFnValue(fr, in.stdFn("std/shell", "stages"), n, []Value{&Ref{Cell: &Cell{V: v}}}, nil)
	if c != nil {
		return nil, c
	}
	p := &hostshell.Plan{}
	for _, sv := range deref(stages).(*Array).Elems {
		rec := deref(sv).(*Record)
		var st hostshell.Stage
		for _, a := range deref(rec.Fields[0]).(*Array).Elems {
			st.Argv = append(st.Argv, str(a))
		}
		for _, r := range deref(rec.Fields[1]).(*Array).Elems {
			rr := deref(r).(*Record)
			st.Redirs = append(st.Redirs, hostshell.Redirect{Op: uint32(deref(rr.Fields[0]).(Int).V), Fd: int(deref(rr.Fields[1]).(Int).V), Target: str(rr.Fields[2])})
		}
		if g := deref(rec.Fields[2]).(*Variant); len(g.Payload) == 1 {
			st.Filter = hostshell.GrepFilter(str(g.Payload[0]))
		}
		p.Stages = append(p.Stages, st)
	}
	return p, nil
}

func registerShell(in *Interp) {
	s := "std/shell."
	in.hostAccel(s+"capture", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		p, c := fr.planOf(n, a[0])
		if c != nil {
			return nil, c
		}
		out, errOut, _, err := hostshell.Run(p, in.cwd)
		if err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		t, _, _ := in.r.Types.IsResult(fr.retType(n))
		return mkOk(in, fr.retType(n), &Record{Type: t, Fields: []Value{Bytes(out), Bytes(errOut)}}), nil
	})
	in.hostAccel(s+"run", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		p, c := fr.planOf(n, a[0])
		if c != nil {
			return nil, c
		}
		out, errOut, statuses, err := hostshell.Run(p, in.cwd)
		if err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		in.stdout.Write(out)
		in.stderr.Write(errOut)
		t, _, _ := in.r.Types.IsResult(fr.retType(n))
		codes := &Array{}
		for _, st := range statuses {
			codes.Elems = append(codes.Elems, Int{V: int64(st), T: sem.TyI64})
		}
		return mkOk(in, fr.retType(n), &Record{Type: t, Fields: []Value{codes}}), nil
	})
}
