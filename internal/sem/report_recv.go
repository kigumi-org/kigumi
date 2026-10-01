package sem

import (
	"strings"

	"kigumi/internal/syntax"
)

// emits E515 for an immutable `mut self` receiver, walking outside-in like reportImmutable
// (E701/E702) to blame the link nearest the point of use.
func (c *checker) reportRecvNotMutable(base syntax.NodeID, name string) {
	fix, tok, file := c.recvMutabilityFix(base)
	if file != 0 && file == c.f && tok != 0 {
		title := "Declare `mut " + c.t.TokText(tok) + "`"
		c.errFix(base, c.r.insertAt(c.f, tok, title, "mut "), cRecvNotMutable, name, c.placeName(base), fix)
		return
	}
	c.errAt(base, cRecvNotMutable, name, c.placeName(base), fix)
}

// a chain can carry more than one independent immutability link; joins them all so the
// suggestion actually makes the receiver mutable, offering the insert-before token only
// for a single link.
func (c *checker) recvMutabilityFix(n syntax.NodeID) (fix string, tok uint32, file FileID) {
	fixes, tok, file := c.recvMutabilityFixes(n)
	switch len(fixes) {
	case 0:
		return "make it mutable", 0, 0
	case 1:
		return fixes[0], tok, file
	default:
		return strings.Join(fixes, ", and "), 0, 0
	}
}

// tok/file name the insert-before token only when the returned fixes has exactly one entry.
func (c *checker) recvMutabilityFixes(n syntax.NodeID) (fixes []string, tok uint32, file FileID) {
	switch c.t.Kind(n) {
	case syntax.Paren, syntax.BracketExpr:
		return c.recvMutabilityFixes(syntax.NodeID(c.t.Nodes[n].Lhs))
	case syntax.MemberExpr:
		rest, rtok, rfile := c.recvMutabilityFixes(syntax.NodeID(c.t.Nodes[n].Lhs))
		if fld := c.info.Uses[n]; fld != 0 && c.r.Entities[fld].Kind == EntField {
			if fe := c.r.Entities[fld]; fe.Flags&EfMut == 0 {
				fix := "declare `mut " + fe.Name + "` on the field"
				return append([]string{fix}, rest...), fe.Tok, fe.File
			}
		}
		return rest, rtok, rfile
	case syntax.Ident:
		root := c.rootOf(n)
		if root == 0 {
			return []string{"make it mutable"}, 0, 0
		}
		e := &c.r.Entities[root]
		if c.r.Types.Kind(e.Type) == KRef {
			// EfMut on the binding is irrelevant here: mutability comes
			// from the reference type, not the local/param itself.
			if c.r.Types.Node(e.Type).Flags&flagMut != 0 {
				return nil, 0, 0
			}
			elem := c.r.Types.Node(e.Type).Elem
			return []string{"annotate `" + e.Name + "` as `&mut " + c.r.TypeString(elem) + "`"}, 0, 0
		}
		if e.Flags&EfMut != 0 || e.Flags&EfSelf != 0 && e.Flags&EfMove != 0 {
			return nil, 0, 0
		}
		if e.Kind == EntParam {
			return []string{"declare `mut " + e.Name + "` in the parameter list"}, e.Tok, e.File
		}
		return []string{"declare `let mut " + e.Name + " = ...`"}, e.Tok, e.File
	}
	return []string{"make it mutable"}, 0, 0
}
