package mir

import (
	"kigumi/internal/diag"
	"kigumi/internal/syntax"
)

// touch reports whether in touches owner, and its field path if so.
// OpField/OpSetField/OpBorrow resolve the full projection so a
// field-chain borrow is checked by field, not by the temp it borrows.
func (c *borrowCheck) touch(in Inst, owner LocalID) (touched bool, path []int, exact bool) {
	switch in.Op {
	case OpField:
		if c.chainedOnly(in.Dst) {
			return false, nil, true
		}
		if root, p, ex, ok := c.projectionPlace(in.Dst); ok && root == owner {
			return true, p, ex
		}
	case OpSetField:
		if root, p, ex, ok := c.projectionPlace(in.Args[0]); ok && root == owner {
			return true, append(p, in.Index), ex
		}
	case OpBorrow:
		if root, p, ex, ok := c.projectionPlace(in.Args[0]); ok && root == owner {
			return true, p, ex
		}
	default:
		if in.Dst == owner && in.Op != OpDrop {
			return true, nil, true
		}
		for _, a := range in.Args {
			if a == owner {
				return true, nil, true
			}
		}
	}
	return false, nil, true
}

func (c *borrowCheck) report(n syntax.NodeID, code, msg string) {
	if n == 0 || c.reported[n] {
		return
	}
	c.reported[n] = true
	t := c.p.R.Tree(c.f.File)
	c.f.Diags = append(c.f.Diags, diag.Diagnostic{Severity: diag.Error, Loc: diag.At(t.File, t.Span(n)), Msg: msg, Code: code})
}
