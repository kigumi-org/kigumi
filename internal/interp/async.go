package interp

import (
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// Future is a lazy call of an async fn; awaiting it runs the body.
type Future struct {
	Fn   sem.EntityID
	Args []Value
}

func (fr *frame) await(n syntax.NodeID, v Value) (Value, *ctrl) {
	f, ok := deref(v).(*Future)
	if !ok {
		fr.panicAt(n, "await on a value that is not a Future")
	}
	args := f.Args
	f.Args = nil
	return fr.in.callFnValue(fr, f.Fn, n, args, nil)
}

// The local executor runs futures on the calling thread: run awaits at
// once, spawn defers until join.
func registerTask(in *Interp) {
	t := "std/task."
	in.def(t+"local", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Executor", nil), nil
	})
	in.def(t+"Executor.run", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return fr.await(n, a[1])
	})
	in.def(t+"Executor.spawn", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Task", deref(a[1])), nil
	})
	in.def(t+"Task.join", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return fr.await(n, deref(a[0]).(*Opaque).Data.(Value))
	})
}
