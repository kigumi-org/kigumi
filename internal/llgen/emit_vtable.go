package llgen

import (
	"fmt"
	"strings"

	"kigumi/internal/sem"
)

// vtable is one adapter per requirement, in requirement order; a boxed
// value carries it so a dynamic call is a slot load.
func (e *emitter) vtable(t, iface sem.TypeID, from sem.PackageID) string {
	name := "@\"vt$" + strings.Trim(strings.TrimPrefix(e.descOfType(t), "@"), "\"") + "$" + e.r.TypeString(iface) + "\""
	if e.cio[name] {
		return name
	}
	e.cio[name] = true
	var entries []string
	for _, w := range e.r.Witnesses(t, iface, from) {
		switch {
		case e.p.ByEnt[w] != nil:
			entries = append(entries, "ptr "+e.adapterSym(e.p.ByEnt[w]))
		default:
			e.stdDrops[w] = true
			entries = append(entries, "ptr "+e.stdAdapterSym(w))
		}
	}
	if len(entries) == 0 {
		entries = []string{"ptr null"}
	}
	fmt.Fprintf(&e.helpers, "%s = global [%d x ptr] [%s]\n\n", name, len(entries), strings.Join(entries, ", "))
	return name
}

func (e *emitter) slotOf(iface, req sem.EntityID) int {
	for i, r := range e.r.Iface(iface).Reqs {
		if r == req {
			return i
		}
	}
	return -1
}
