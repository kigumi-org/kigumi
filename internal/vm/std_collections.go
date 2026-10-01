package vm

// collection implements the array primitives; nil for other keys.
func (m *Machine) collection(key string, a []*obj, a0, a1, a2 *obj) *obj {
	switch key {
	case "array.Array.empty":
		return mkArray(nil)
	case "array.Array.of":
		return retain(a0)
	case "array.Array.push":
		a0.fields = append(a0.fields, retain(a[1]))
		// Growth happens in place on a0, not on the Unit this returns, so
		// the sandbox's per-call chargeMemory(result) never sees it.
		m.chargeMemory(a0)
		return unitObj
	case "array.Array.growBy":
		return mkBool(uint64(a1.i) < 1<<40)
	case "array.Array.len":
		return mkInt(int64(len(a0.fields)), 64)
	case "array.Array.get":
		if a1.i < 0 || a1.i >= int64(len(a0.fields)) {
			return m.none()
		}
		return m.some(m.copy(a0.fields[a1.i]))
	case "array.Array.with":
		if a1.i < 0 || a1.i >= int64(len(a0.fields)) {
			return m.none()
		}
		return m.some(m.callValue(a2, []*obj{a0.fields[a1.i]}))
	case "array.Array.withMut":
		if a1.i < 0 || a1.i >= int64(len(a0.fields)) {
			return m.none()
		}
		// A scalar element aliases its obj on copy, so the callback
		// writes through a cell instead; the slot takes it back after.
		cell := mkCell(a0.fields[a1.i])
		r := m.callValue(a2, []*obj{cell})
		a0.fields[a1.i] = cell.inner
		return m.some(r)
	case "array.Array.takeAt":
		i := a1.i
		if i < 0 || i >= int64(len(a0.fields)) {
			return m.none()
		}
		x := a0.fields[i]
		a0.fields = append(a0.fields[:i], a0.fields[i+1:]...)
		m.chargeMemory(a0)
		return m.some(x)
	case "array.Array.clone":
		return m.copy(a0)
	}
	return nil
}
