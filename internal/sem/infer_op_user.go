package sem

import (
	"strings"

	"kigumi/internal/syntax"
)

func (c *checker) userOperator(n syntax.NodeID, op string, operands []TypeID, nodes []syntax.NodeID) TypeID {
	tt := c.r.Types
	var matches []EntityID
	seen := map[EntityID]bool{}
	for _, t := range operands {
		t = c.vars.resolve(t)
		if tt.Kind(t) != KNamed {
			continue
		}
		set, ok := c.r.typeDecl(tt.Node(t).Ent).Ops[op]
		if !ok {
			continue
		}
		for _, m := range c.r.Overloads[set].Members {
			if seen[m] || !c.r.visibleFrom(c.r.Entities[m].Vis, c.pkg) {
				continue
			}
			seen[m] = true
			if c.operatorMatches(m, operands, nodes) {
				matches = append(matches, m)
			}
		}
	}
	switch len(matches) {
	case 1:
		return c.callOperator(n, op, matches[0], operands, nodes)
	case 0:
		return c.operatorUndefined(n, op, operands, nodes)
	}
	owners := ""
	for i, m := range matches[:2] {
		if i > 0 {
			owners += "` and `"
		}
		owners += c.r.entityName(c.r.Fn(m).Owner)
	}
	c.errAt(n, cOperatorAmbiguous, op, c.typesText(operands), owners, "")
	return TyPoison
}

func (c *checker) operatorMatches(m EntityID, operands []TypeID, nodes []syntax.NodeID) bool {
	tt := c.r.Types
	params := tt.Node(c.r.Fn(m).Sig).Args
	if len(params) != len(operands) {
		return false
	}
	shapes := make([]TypeID, len(operands))
	for i, t := range operands {
		t = c.vars.resolve(t)
		if tt.Kind(t) == KUntyped {
			if tt.IsNumeric(params[i]) && c.fitsQuiet(c.literalNode(nodes[i]), params[i]) {
				t = params[i]
			} else {
				t = defaultOf(t)
			}
		}
		shapes[i] = t
	}
	return c.trialMatch(m, 0, shapes)
}

func (c *checker) callOperator(n syntax.NodeID, op string, m EntityID, operands []TypeID, nodes []syntax.NodeID) TypeID {
	tt := c.r.Types
	params := tt.Node(c.r.Fn(m).Sig).Args
	for i, t := range operands {
		if tt.Kind(c.vars.resolve(t)) == KUntyped && tt.IsNumeric(params[i]) {
			c.adopt(c.literalNode(nodes[i]), params[i])
		}
	}
	ret := c.callFn(n, m, nil, nodes, 0, 0)
	call := c.info.Calls[n]
	call.Kind = CallOperator
	call.OpText = op
	c.info.Calls[n] = call
	return ret
}

// A built-in operator followed by a prefix operator suggests a missing space.
func (c *checker) operatorUndefined(n syntax.NodeID, op string, operands []TypeID, nodes []syntax.NodeID) TypeID {
	tt := c.r.Types
	for i, t := range operands {
		if tt.Kind(c.vars.resolve(t)) == KUntyped {
			operands[i] = c.adopt(c.literalNode(nodes[i]), defaultOf(c.vars.resolve(t)))
		}
	}
	if len(operands) == 2 {
		if last := op[len(op)-1]; len(op) >= 2 && strings.IndexByte("-!~", last) >= 0 && isBuiltinOp(op[:len(op)-1]) {
			c.errAt(n, cOperatorSpacingHint, op, c.placeName(nodes[0]), op[:len(op)-1], string(last), c.placeName(nodes[1]))
			return TyPoison
		}
		return c.operandMismatch(n, nodes[1], op, operands[0], operands[1])
	}
	c.errAt(n, cOperatorUndefined, op, c.typesText(operands))
	return TyPoison
}

func isBuiltinOp(s string) bool {
	switch s {
	case "+", "-", "*", "/", "%", "<", ">", "<=", ">=", "==", "!=", "&", "|", "^", "<<", ">>":
		return true
	}
	return false
}
