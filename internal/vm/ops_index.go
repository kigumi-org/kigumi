package vm

func (m *Machine) index(base, idx *obj) *obj {
	base, idx = deref(base), deref(idx)
	if idx.k == kRecord {
		lo, hi := deref(idx.fields[0]).i, deref(idx.fields[1]).i
		if deref(idx.fields[2]).b {
			hi++
		}
		if base.k == kStr || base.k == kBytes {
			if lo < 0 || hi > int64(len(base.s)) || lo > hi {
				m.abort("slice is not on character boundaries")
			}
			if base.k == kStr && (!boundary(base.s, lo) || !boundary(base.s, hi)) {
				m.abort("slice is not on character boundaries")
			}
			out := newObj(base.k)
			out.s = append([]byte{}, base.s[lo:hi]...)
			m.chargeMemory(out)
			return out
		}
		if lo < 0 || hi > int64(len(base.fields)) || lo > hi {
			m.abort("slice out of range")
		}
		out := mkArray(base.fields[lo:hi])
		for _, x := range out.fields {
			retain(x)
		}
		m.chargeMemory(out)
		return out
	}
	i := idx.i
	if base.k == kStr || base.k == kBytes {
		if i < 0 || i >= int64(len(base.s)) {
			m.abort("index out of range")
		}
		return mkInt(int64(base.s[i]), 8)
	}
	if base.k != kArray || i < 0 || i >= int64(len(base.fields)) {
		m.abort("index out of range")
	}
	return retain(base.fields[i])
}

func (m *Machine) setIndex(base, idx, v *obj) {
	base = deref(base)
	i := deref(idx).i
	if base.k != kArray || i < 0 || i >= int64(len(base.fields)) {
		m.abort("index out of range")
	}
	old := base.fields[i]
	base.fields[i] = v
	m.release(old)
}
