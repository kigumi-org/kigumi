package mir

import (
	"strings"

	"kigumi/internal/sem"
)

// StdKey names a bodiless std function the way the runtime's dispatch
// tables key it; stripStd drops the "std/" prefix for the VM's table,
// since llgen's own C string reaches rt_std, which strips it itself.
func StdKey(r *sem.Result, fn sem.EntityID, stripStd bool) string {
	ent := r.Entity(fn)
	info := r.Fn(fn)
	key := r.Packages[ent.Pkg].Path + "."
	if stripStd {
		key = strings.TrimPrefix(key, "std/")
	}
	if info.Owner != 0 {
		key += r.Entity(info.Owner).Name + "."
	}
	return key + ent.Name
}
