package sem

import (
	"strings"

	"kigumi/internal/syntax"
)

// checkPattern types a pattern against the scrutinee type and declares its
// bindings in scope. mut applies to top-level bindings.
func (c *checker) checkPattern(p syntax.NodeID, scrutinee TypeID, mut bool, scope ScopeID) PatInfo {
	return c.checkPatternOn(p, 0, scrutinee, mut, scope)
}

func (c *checker) patternInner(p syntax.NodeID, scrutinee TypeID, mut bool, seen map[string]syntax.NodeID) PatInfo {
	node := c.t.Nodes[p]
	tt := c.r.Types
	c.setType(p, scrutinee)
	switch node.Kind {
	case syntax.PatWildcard:
		return PatInfo{Kind: PatWild, Type: scrutinee}
	case syntax.PatBind:
		name := c.t.TokText(node.Tok)
		if prev, dup := seen[name]; dup && prev != p {
			c.errAt(p, cPatternDuplicateBinding, name)
		}
		seen[name] = p
		if scrutinee != TyPoison && c.shadowsVariant(scrutinee, name) {
			c.errAt(p, cBindingShadowsVariant, name, c.r.entityName(tt.Node(scrutinee).Ent), name)
		}
		c.declareLocal(p, name, c.bindingType(scrutinee), mut)
		return PatInfo{Kind: PatBinding, Type: scrutinee, Moves: !c.patBorrow && !c.r.isCopy(scrutinee)}
	case syntax.PatLit:
		lit := syntax.NodeID(node.Lhs)
		return c.literalPattern(p, lit, scrutinee)
	case syntax.PatCtor:
		return c.ctorPattern(p, scrutinee, seen)
	case syntax.PatRecord:
		return c.recordPattern(p, scrutinee, seen)
	case syntax.PatTuple:
		return c.tuplePattern(p, scrutinee, seen)
	case syntax.PatRange:
		return c.rangePattern(p, scrutinee)
	case syntax.PatOr:
		reported := false
		for _, alt := range c.t.Children(p) {
			c.patternInner(alt, scrutinee, mut, map[string]syntax.NodeID{})
			if b := firstBinding(c.t, alt); b != 0 && !reported {
				c.errAt(b, cOrPatternBinding, c.t.TokText(c.t.Nodes[b].Tok))
				reported = true
			}
		}
		return PatInfo{Kind: PatWild, Type: scrutinee}
	}
	c.errAt(p, cPatternKind, scrutinee)
	return PatInfo{Kind: PatWild, Type: scrutinee}
}

func (c *checker) shadowsVariant(scrutinee TypeID, name string) bool {
	n := c.r.Types.Node(scrutinee)
	if n.Kind != KNamed {
		return false
	}
	for _, v := range c.r.typeDecl(n.Ent).Variants {
		if c.r.Entities[v].Name == name && len(c.r.variant(v).Payload) == 0 {
			return true
		}
	}
	return false
}

func (c *checker) literalPattern(p, lit syntax.NodeID, scrutinee TypeID) PatInfo {
	tt := c.r.Types
	got := c.vars.resolve(c.synth(lit))
	switch {
	case scrutinee == TyPoison || got == TyPoison:
	case tt.Kind(got) == KUntyped:
		if !tt.IsNumeric(scrutinee) {
			c.errAt(p, cPatternLiteralType, c.t.TokText(c.t.Nodes[c.literalNode(lit)].Tok), scrutinee)
			c.setType(p, TyPoison)
		} else {
			c.adopt(c.literalNode(lit), scrutinee)
		}
	case got != scrutinee:
		c.errAt(p, cPatternLiteralType, c.t.TokText(c.t.Nodes[c.literalNode(lit)].Tok), scrutinee)
		c.setType(p, TyPoison)
	}
	return PatInfo{Kind: PatLiteral, Type: scrutinee}
}

// irrefutable is the syntactic test.
func (c *checker) irrefutable(p syntax.NodeID, scrutinee TypeID) bool {
	node := c.t.Nodes[p]
	switch node.Kind {
	case syntax.PatWildcard, syntax.PatBind:
		return true
	case syntax.PatRecord:
		info, ok := c.info.Pats[p]
		if ok && info.Kind != PatRecordTy {
			return false
		}
		for _, f := range c.t.Children(syntax.NodeID(node.Rhs)) {
			if sub := syntax.NodeID(c.t.Nodes[f].Lhs); sub != 0 && !c.irrefutable(sub, 0) {
				return false
			}
		}
		return true
	case syntax.PatTuple:
		for _, s := range c.t.Children(p) {
			if !c.irrefutable(s, 0) {
				return false
			}
		}
		return true
	}
	return false
}

func (c *checker) witnessText(p syntax.NodeID, scrutinee TypeID) string {
	node := c.t.Nodes[p]
	scrutinee = c.scrutineeOf(scrutinee)
	switch node.Kind {
	case syntax.PatCtor:
		if v := c.info.Uses[p]; v != 0 && c.r.Entities[v].Kind == EntVariant {
			var others []string
			for _, o := range c.r.typeDecl(c.r.Entities[v].Parent).Variants {
				if o != v {
					others = append(others, c.r.Entities[o].Name+c.arityHoles(o))
				}
			}
			if len(others) > 0 {
				return strings.Join(others, " | ")
			}
		}
	case syntax.PatLit:
		return "_"
	}
	return "_"
}

func (c *checker) arityHoles(v EntityID) string {
	n := len(c.r.variant(v).Payload)
	if n == 0 {
		return ""
	}
	return "(" + strings.TrimSuffix(strings.Repeat("_, ", n), ", ") + ")"
}

func firstBinding(t *syntax.Tree, p syntax.NodeID) syntax.NodeID {
	switch t.Kind(p) {
	case syntax.PatBind:
		return p
	case syntax.PatCtor:
		if rhs := syntax.NodeID(t.Nodes[p].Rhs); rhs != 0 {
			for _, s := range t.Children(rhs) {
				if b := firstBinding(t, s); b != 0 {
					return b
				}
			}
		}
	case syntax.PatRecord:
		for _, f := range t.Children(syntax.NodeID(t.Nodes[p].Rhs)) {
			sub := syntax.NodeID(t.Nodes[f].Lhs)
			if sub == 0 {
				return f
			}
			if b := firstBinding(t, sub); b != 0 {
				return b
			}
		}
	case syntax.PatTuple:
		for _, s := range t.Children(p) {
			if b := firstBinding(t, s); b != 0 {
				return b
			}
		}
	case syntax.PatOr:
		for _, alt := range t.Children(p) {
			if b := firstBinding(t, alt); b != 0 {
				return b
			}
		}
	}
	return 0
}
