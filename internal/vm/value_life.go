package vm

// boxReplace overwrites dst's content with v's, keeping dst's own identity
// (its rc stays put) so every other holder of dst sees the new value; what
// ends up in v is dst's old content, released the way any value is.
func (m *Machine) boxReplace(dst, v *obj) {
	dstRC, vRC := dst.rc, v.rc
	*dst, *v = *v, *dst
	dst.rc, v.rc = dstRC, vRC
	m.release(v)
}

// release drops one reference; the machine runs the drop hook and frees
// the children when it was the last.
func (m *Machine) release(v *obj) {
	if v == nil || v.rc <= 0 {
		return
	}
	v.rc--
	if v.rc == 0 {
		m.free(v)
	}
}

func (m *Machine) free(v *obj) {
	m.unchargeMemory(v)
	switch v.k {
	case kRecord:
		v.rc = -1
		// Priority matches callEntity (calls.go): the host accelerator
		// wins over a lowered body unless NoAccel asks for the real one.
		drop := m.dropFn(v.ent)
		if h, ent := m.hostDrop(v.ent); h != nil && (drop == nil || !m.opts.NoAccel) {
			h(m, ent, []*obj{v})
		} else if drop != nil {
			m.invoke(drop, nil, false, []*obj{v})
		}
		v.rc = 0
		for i := len(v.fields) - 1; i >= 0; i-- {
			m.release(v.fields[i])
		}
	case kVariant:
		for _, p := range v.fields {
			m.release(p)
		}
	case kArray:
		for i := len(v.fields) - 1; i >= 0; i-- {
			m.release(v.fields[i])
		}
	case kBox, kCell:
		m.release(v.inner)
	case kClosure:
		m.release(v.env)
	case kOpaque:
		m.freeOpaque(v)
	}
	// A freed object is poisoned so a later use is caught, like -DKIGUMI_RC_CHECK.
	*v = obj{k: kFree}
}

// copy duplicates a value the way rt_copy does: structural values deeply,
// resources and everything else by reference. Each level charges its own
// approxSize before returning, not only the outermost caller, since
// approxSize is shallow (a spine, not a subtree) (alloc_budget.go).
func (m *Machine) copy(v *obj) *obj {
	if v == nil {
		return v
	}
	switch v.k {
	case kRecord:
		if m.isResource(v.ent) {
			return retain(v)
		}
		c := mkRecord(v.ent, v.fields)
		for i := range c.fields {
			c.fields[i] = m.copy(c.fields[i])
		}
		m.chargeMemory(c)
		return c
	case kVariant:
		c := mkVariant(v.ent, v.fields)
		for i := range c.fields {
			c.fields[i] = m.copy(c.fields[i])
		}
		m.chargeMemory(c)
		return c
	case kArray:
		c := mkArray(v.fields)
		for i := range c.fields {
			c.fields[i] = m.copy(c.fields[i])
		}
		m.chargeMemory(c)
		return c
	case kBox:
		c := newObj(kBox)
		c.dyn, c.inner = v.dyn, m.copy(v.inner)
		m.chargeMemory(c)
		return c
	}
	return retain(v)
}
