package interp

import (
	"kigumi/internal/netsock"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// std/net's Conn/Listener bodies reach the host through extern(C) socket
// calls the interpreter cannot make; these are their Go counterparts,
// sharing netsock's raw-fd implementation with the VM's. A Conn/Listener
// is record{fd}, so fdOf/connRecord read and build that one field.
func registerNetAccel(in *Interp) {
	p := "std/net."
	fdOf := func(v Value) int {
		return int(deref(deref(v).(*Record).Fields[0]).(Int).V)
	}
	connRecord := func(t sem.TypeID, fd int) Value {
		return &Record{Type: t, Fields: []Value{Int{V: int64(fd), T: sem.TyI32}}}
	}
	dialOrListen := func(dial func(string, int64) (int, error)) builtinFn {
		return func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
			host := string(deref(a[1]).(Str))
			port := deref(a[2]).(Int).V
			fd, err := dial(host, port)
			if err != nil {
				return mkErr(in, fr.retType(n), err.Error()), nil
			}
			t, _, _ := in.r.Types.IsResult(fr.retType(n))
			return mkOk(in, fr.retType(n), connRecord(t, fd)), nil
		}
	}
	in.hostAccel(p+"Net.connect", dialOrListen(netsock.Dial))
	in.hostAccel(p+"Net.listen", dialOrListen(netsock.Listen))
	in.hostAccel(p+"Listener.accept", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		fd, err := netsock.Accept(fdOf(a[0]))
		if err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		t, _, _ := in.r.Types.IsResult(fr.retType(n))
		return mkOk(in, fr.retType(n), connRecord(t, fd)), nil
	})
	in.hostAccel(p+"Listener.port", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return Int{V: int64(netsock.Port(fdOf(a[0]))), T: sem.TyI64}, nil
	})
	in.hostAccel(p+"Conn.read", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		max := deref(a[2]).(Int).V
		if max < 0 {
			return mkErr(in, fr.retType(n), "out of memory"), nil
		}
		buf := make([]byte, max)
		got, err := netsock.Read(fdOf(a[0]), buf)
		if err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		arr := deref(a[1]).(*Array)
		for _, b := range buf[:got] {
			arr.Elems = append(arr.Elems, Int{V: int64(b), T: sem.TyU8})
		}
		return mkOk(in, fr.retType(n), usize(got)), nil
	})
	in.hostAccel(p+"Conn.write", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		sent, err := netsock.Write(fdOf(a[0]), []byte(deref(a[1]).(Bytes)))
		if err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		return mkOk(in, fr.retType(n), usize(sent)), nil
	})
	in.hostAccel(p+"Conn.close", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		netsock.Close(fdOf(a[0]))
		return mkOk(in, fr.retType(n), Unit{}), nil
	})
	// Bodiless (net_hosted.kg's Conn.drop/Listener.drop have no body): def,
	// not hostAccel, since a bodiless callee looks itself up in builtins.
	in.def(p+"Conn.drop", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		netsock.Close(fdOf(a[0]))
		return Unit{}, nil
	})
	in.def(p+"Listener.drop", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		netsock.Close(fdOf(a[0]))
		return Unit{}, nil
	})
}
