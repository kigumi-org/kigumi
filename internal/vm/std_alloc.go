package vm

// sharedCtrl is the control block behind Shared and Weak: the cell holding
// the value, whether a strong reference still exists, and the weak count.
// borrow is a RefCell-style dynamic borrow flag (0 free, -1 exclusive, >0
// shared count) since Shared's receiver is a freely-copyable handle the
// static borrow checker does not track.
type sharedCtrl struct {
	cell   *obj
	shared *obj
	weak   int
	alive  bool
	borrow int
}

// alloc implements std/alloc: Shared / Weak by a control block, arenas as
// plain handles (the VM allocates from Go, so allocated() reports 0).
func (m *Machine) alloc(key string, a []*obj, a0, a1 *obj) *obj {
	switch key {
	case "alloc.Arena.create":
		return m.ok(mkOpaque("AllocatorHandle", nil))
	case "alloc.AllocatorHandle.scope":
		return mkOpaque("AllocatorScope", retain(a0))
	case "alloc.AllocatorHandle.allocated":
		return mkInt(0, 64|256)
	case "alloc.Shared.new":
		c := &sharedCtrl{cell: mkCell(retain(a0)), alive: true}
		c.shared = mkOpaque("Shared", c)
		return c.shared
	case "alloc.Shared.clone", "alloc.Weak.clone":
		return retain(a0)
	case "alloc.Shared.get":
		return m.copy(a0.data.(*sharedCtrl).cell.inner)
	case "alloc.Shared.set":
		cell := a0.data.(*sharedCtrl).cell
		old := cell.inner
		cell.inner = retain(a1)
		m.release(old)
		return unitObj
	case "alloc.Shared.with":
		c := a0.data.(*sharedCtrl)
		if c.borrow < 0 {
			m.abort("Shared is exclusively borrowed")
		}
		c.borrow++
		defer func() { c.borrow-- }()
		return m.callValue(a1, []*obj{c.cell.inner})
	case "alloc.Shared.withMut":
		c := a0.data.(*sharedCtrl)
		if c.borrow != 0 {
			m.abort("Shared is already borrowed")
		}
		c.borrow = -1
		defer func() { c.borrow = 0 }()
		// The callback writes through c.cell itself, same as any
		// other &mut Cell store, instead of a snapshot of its content.
		return m.callValue(a1, []*obj{c.cell})
	case "alloc.Shared.downgrade":
		c := a0.data.(*sharedCtrl)
		c.weak++
		return mkOpaque("Weak", c)
	case "alloc.Weak.upgrade":
		c := a0.data.(*sharedCtrl)
		if c.alive {
			return m.some(retain(c.shared))
		}
		return m.none()
	}
	return nil
}

// freeOpaque releases what an opaque handle owns.
func (m *Machine) freeOpaque(v *obj) {
	switch v.op {
	case "Shared":
		c := v.data.(*sharedCtrl)
		c.alive = false
		m.release(c.cell)
	case "Weak":
		c := v.data.(*sharedCtrl)
		c.weak--
	case "AllocatorScope", "Task":
		if h, ok := v.data.(*obj); ok {
			m.release(h)
		}
	case "Future":
		f := v.data.(*future)
		m.release(f.fn)
		for _, a := range f.args {
			m.release(a)
		}
	}
}
