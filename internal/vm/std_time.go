package vm

import (
	"time"

	"kigumi/internal/sem"
)

// timeMonotonicStart anchors time.monotonic(): time.Since keeps using the
// monotonic reading Go's time.Now carries, immune to wall-clock adjustments.
var timeMonotonicStart = time.Now()

func (m *Machine) registerTime() {
	h := m.host
	h["time.now"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		return mkRecord(m.r.Types.Node(m.retType(fn)).Ent, []*obj{mkInt(time.Now().UnixMilli(), m.numKind(sem.TyI64))})
	}
	h["time.sleep"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		time.Sleep(time.Duration(deref(a[0]).i) * time.Millisecond)
		return unitObj
	}
	h["time.monotonic"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		return mkRecord(m.r.Types.Node(m.retType(fn)).Ent, []*obj{mkInt(time.Since(timeMonotonicStart).Milliseconds(), m.numKind(sem.TyI64))})
	}
}
