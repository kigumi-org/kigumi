package sem

import (
	"strings"

	"kigumi/internal/syntax"
)

// emits E701/E702 for an OWN-6 violation, walking outside-in like recvMutabilityFixes (E515)
// to blame the link nearest the point of use.
func (c *checker) reportImmutable(lhs syntax.NodeID) {
	node := c.t.Nodes[lhs]
	switch node.Kind {
	case syntax.Ident:
		if root := c.rootOf(lhs); root != 0 {
			name := c.t.TokText(node.Tok)
			fix := "declare `let mut " + name + "`"
			if e := c.r.Entities[root]; e.File == c.f && e.Tok != 0 {
				c.errFix(lhs, c.r.insertAt(c.f, e.Tok, "Declare `mut "+name+"`", "mut "), cAssignImmutable, name, fix)
				return
			}
			c.errAt(lhs, cAssignImmutable, name, fix)
			return
		}
	case syntax.MemberExpr:
		if fld := c.info.Uses[lhs]; fld != 0 && c.r.Entities[fld].Kind == EntField {
			if fe := c.r.Entities[fld]; fe.Flags&EfMut == 0 {
				fix, tok, file := c.assignImmutableFix(lhs)
				if file != 0 && file == c.f && tok != 0 {
					c.errFix(lhs, c.r.insertAt(c.f, tok, "Declare `mut "+fe.Name+"`", "mut "), cAssignImmutableField, fe.Name, fix)
					return
				}
				c.errAt(lhs, cAssignImmutableField, fe.Name, fix)
				return
			}
			c.reportImmutable(syntax.NodeID(node.Lhs))
			return
		}
	case syntax.BracketExpr:
		if c.isPlace(lhs) {
			c.reportImmutable(syntax.NodeID(node.Lhs))
			return
		}
	}
	c.errAt(lhs, cAssignNotPlace)
}

// mirrors recvMutabilityFix (E515): an insert-before token is offered only for a single link.
func (c *checker) assignImmutableFix(n syntax.NodeID) (fix string, tok uint32, file FileID) {
	fixes, tok, file := c.assignImmutableFixes(n)
	switch len(fixes) {
	case 0:
		return "make it mutable", 0, 0
	case 1:
		return fixes[0], tok, file
	default:
		return strings.Join(fixes, ", and "), 0, 0
	}
}

// walks n's place chain outside-in for every OWN-6-broken link (nearest point of use first); a
// chain can carry both a non-mut field and an immutable root, so it must not stop at the first.
func (c *checker) assignImmutableFixes(n syntax.NodeID) (fixes []string, tok uint32, file FileID) {
	switch c.t.Kind(n) {
	case syntax.Paren, syntax.BracketExpr:
		return c.assignImmutableFixes(syntax.NodeID(c.t.Nodes[n].Lhs))
	case syntax.MemberExpr:
		rest, rtok, rfile := c.assignImmutableFixes(syntax.NodeID(c.t.Nodes[n].Lhs))
		if fld := c.info.Uses[n]; fld != 0 && c.r.Entities[fld].Kind == EntField {
			if fe := c.r.Entities[fld]; fe.Flags&EfMut == 0 {
				fix := "declare `mut " + fe.Name + "`"
				return append([]string{fix}, rest...), fe.Tok, fe.File
			}
		}
		return rest, rtok, rfile
	case syntax.Ident:
		root := c.rootOf(n)
		if root == 0 || c.isMutablePlace(n) {
			return nil, 0, 0
		}
		e := &c.r.Entities[root]
		return []string{"declare `let mut " + e.Name + "`"}, e.Tok, e.File
	}
	return nil, 0, 0
}
