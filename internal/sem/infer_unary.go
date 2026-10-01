package sem

import (
	"kigumi/internal/diag"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func (c *checker) synthUnary(n syntax.NodeID, want TypeID) TypeID {
	node := c.t.Nodes[n]
	op := c.t.Toks[node.Tok].Kind
	if isNumericLiteral(c.t, n) {
		return c.synthLiteral(n, want)
	}
	operand := c.synth(syntax.NodeID(node.Lhs))
	tt := c.r.Types
	operand = c.readThrough(c.vars.resolve(operand))
	if operand == TyPoison {
		return TyPoison
	}
	if tt.Kind(operand) == KUntyped {
		operand = c.adopt(c.literalNode(syntax.NodeID(node.Lhs)), defaultOf(operand))
	}
	switch op {
	case token.Minus:
		if tt.IsSigned(operand) {
			return operand
		}
		if tt.IsInteger(operand) {
			c.errAt(n, cUnaryMinusUnsigned, operand)
			return TyPoison
		}
	case token.Bang:
		if operand == TyBool {
			return TyBool
		}
	case token.Tilde:
		if tt.IsInteger(operand) {
			return operand
		}
	}
	return c.userOperator(n, c.t.TokText(node.Tok), []TypeID{operand}, []syntax.NodeID{syntax.NodeID(node.Lhs)})
}

func (c *checker) synthBorrow(n syntax.NodeID, want TypeID) TypeID {
	node := c.t.Nodes[n]
	inner := syntax.NodeID(node.Rhs)
	t := c.synth(inner)
	if w := c.r.Types.Node(c.vars.resolve(want)); w.Kind == KRef && c.r.Types.Kind(c.vars.resolve(t)) == KUntyped {
		target := c.vars.resolve(w.Elem)
		if c.r.Types.ContainsVar(target) || c.r.Types.Kind(target) == KUntyped {
			target = defaultOf(c.vars.resolve(t))
		}
		t = c.adopt(c.literalNode(inner), target)
	}
	mut := node.Lhs&syntax.FlagMut != 0
	if mut && c.indexAssignUnsupported(inner) {
		c.errAt(inner, cIndexAssignUnsupported)
	} else if mut && !c.isMutablePlace(inner) {
		c.errAt(inner, cAssignNotPlace)
	}
	if mut {
		c.invalidatePlace(inner)
	}
	refT := c.r.Types.Ref(t, mut)
	// A scalar `&mut` of a bare local/parameter can be stored (MIR
	// build_expr.go hir.Borrow): flag the local so it gets a cell to alias
	// into, instead of the borrow copying a disconnected snapshot.
	if mut {
		if _, ok := c.r.ScalarRefElem(refT); ok {
			if ent, ok := c.identPlaceEnt(inner); ok {
				c.r.Entities[ent].Flags |= EfAddrTaken
				// Writes go through this borrow, not ent, so promote the
				// capture here instead of relying on assignStmt.
				c.noteCaptureWrite(inner, ent)
			}
		}
	}
	return refT
}

func (c *checker) noteDiag(d diag.Diagnostic) { c.r.emitDiag(c.f, d) }
