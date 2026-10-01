package sem

// reportUnused warns about imports no body referenced. It stays silent until
// bodies are checked, since only body checking marks imports as used.
func (r *Result) reportUnused() {
	if !r.bodiesChecked {
		return
	}
	for id := 1; id < len(r.Entities); id++ {
		e := &r.Entities[id]
		if e.Kind != EntImport || e.Flags&(EfUsed|EfReexport|EfPoison) != 0 {
			continue
		}
		r.errFix(e.File, e.Node, r.removeLine(e.File, e.Node, "Remove unused import"), cImportUnused, e.Name)
	}
}
