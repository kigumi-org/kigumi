package vm

// objOverhead approximates one obj allocation's fixed bookkeeping cost.
const objOverhead = 32

// approxSize estimates the bytes a value's own backing storage costs right
// now. Scalars (int/float/bool/char/unit) count as 0: the sandbox's memory
// budget bounds growing data (strings, bytes, arrays, records, closures),
// not the interpreter's per-instruction temporaries, so a tight arithmetic
// loop still fails on comptimeStepLimit first (frame.go), matching the
// sandbox's separate step/memory/depth axes.
func approxSize(v *obj) int64 {
	if v == nil {
		return 0
	}
	switch v.k {
	case kStr, kBytes:
		return int64(len(v.s)) + objOverhead
	case kArray, kRecord, kVariant, kClosure:
		return int64(len(v.fields))*8 + objOverhead
	case kCell, kBox, kOpaque:
		return objOverhead
	}
	return 0
}

// chargeMemory brings v's share of the sandboxed run's live total up to
// date and aborts once it crosses Options.MemoryLimit; 0 (EvalBuild)
// leaves allocation unbounded. v.chg holds the last-charged byte count, not
// a one-shot flag, so re-charging the same size is a no-op and a primitive
// that grows a live receiver in place (Array.push) can bill the growth too.
func (m *Machine) chargeMemory(v *obj) {
	if !m.opts.Sandbox || m.opts.MemoryLimit <= 0 || v == nil {
		return
	}
	size := approxSize(v)
	delta := size - v.chg
	if delta == 0 {
		return
	}
	v.chg = size
	m.mem += delta
	if m.mem < 0 {
		m.mem = 0
	}
	if delta > 0 && m.mem > m.opts.MemoryLimit {
		m.abort("comptime block allocated too much memory")
	}
}

// unchargeMemory reclaims a freed value's last-charged share of the live
// total. m.mem tracks live bytes, not cumulative churn, so a loop that
// builds and immediately drops small values stays bounded instead of
// tripping the limit on volume alone.
func (m *Machine) unchargeMemory(v *obj) {
	if !m.opts.Sandbox || m.opts.MemoryLimit <= 0 || v == nil || v.chg == 0 {
		return
	}
	m.mem -= v.chg
	if m.mem < 0 {
		m.mem = 0
	}
}
