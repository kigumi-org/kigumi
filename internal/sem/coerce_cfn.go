package sem

import "kigumi/internal/syntax"

// coerceToCFn converts a function item into a C function pointer.
// Only export(C) qualifies; extern declarations already carry the pointer type.
func (c *checker) coerceToCFn(n syntax.NodeID, got, want TypeID) ([]CoStep, bool) {
	tt := c.r.Types
	g := tt.Node(got)
	if g.Kind == KFn && g.Flags&fnCAbi != 0 {
		return nil, false
	}
	item := c.info.Uses[n]
	if g.Kind != KFn || item == 0 || c.r.Entities[item].Kind != EntFn || c.r.Fn(item).Export == "" {
		c.errAt(n, cCFnPtrSource, c.r.TypeString(want))
		return nil, true
	}
	if g.Flags&fnVariadic != 0 || tt.Node(want).Flags&fnCVariadic != 0 || !c.sameFnShape(got, want) {
		return nil, false
	}
	return []CoStep{{CoCFnPtr, want}}, true
}
