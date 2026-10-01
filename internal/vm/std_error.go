package vm

import "kigumi/internal/sem"

// registerError implements std/error.as[E]: `e is E(x)` doesn't type-check
// over a type parameter (E101), so this downcast compares box
// identity the way OpIsType does for a concrete type, looking through any
// Context wrapping. sem requires E: Copy, so the match is duplicated with
// m.copy (like Shared[T: Copy].get), never aliased into the caller's borrow.
func (m *Machine) registerError() {
	m.host["error.as"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		opt, _ := m.r.Types.IsOption(m.siteFn.Locals[m.site.Dst].Type)
		want := m.r.Types.Node(opt).Ent
		ctxEnt := m.r.LangItem("Context")
		cur := deref(a[0])
		owned := false
		for {
			if cur.k != kBox {
				break
			}
			et := m.typeEnt(cur.dyn)
			if et == want {
				v := m.copy(cur.inner)
				if owned {
					m.release(cur)
				}
				return m.some(v)
			}
			if ctxEnt == 0 || et != ctxEnt {
				break
			}
			idx := m.r.Field(m.r.FindField(ctxEnt, "wrapped")).Index
			next := deref(m.field(cur, idx))
			if owned {
				m.release(cur)
			}
			cur, owned = next, true
		}
		if owned {
			m.release(cur)
		}
		return m.none()
	}
}
