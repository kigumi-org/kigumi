package vm

import "kigumi/internal/sem"

// future holds a lazy async call; like the C runtime it keeps its
// arguments until the Future value itself is dropped, so awaiting twice
// is a plain error of the program rather than a use after free.
type future struct {
	fn   *obj
	args []*obj
}

// asyncBuiltin implements `future` and `await`: the local executor runs
// futures on the calling thread, as the interpreter does.
func (m *Machine) asyncBuiltin(name string, args []*obj) (*obj, bool) {
	switch name {
	case "future":
		for _, a := range args {
			retain(a)
		}
		return mkOpaque("Future", &future{fn: args[0], args: append([]*obj{}, args[1:]...)}), true
	case "await":
		return m.await(args[0]), true
	}
	return nil, false
}

func (m *Machine) await(v *obj) *obj {
	v = deref(v)
	f, ok := v.data.(*future)
	if v.k != kOpaque || !ok {
		m.abort("await on a value that is not a Future")
	}
	args := make([]*obj, len(f.args))
	for i, a := range f.args {
		args[i] = retain(a)
	}
	return m.callValue(f.fn, args)
}

func (m *Machine) registerTask() {
	h := m.host
	h["task.local"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj { return mkOpaque("Executor", nil) }
	h["task.Executor.run"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj { return m.await(a[1]) }
	h["task.Executor.spawn"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj { return mkOpaque("Task", retain(deref(a[1]))) }
	h["task.Task.join"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj { return m.await(deref(a[0]).data.(*obj)) }
}
