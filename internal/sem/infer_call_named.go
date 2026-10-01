package sem

import (
	"slices"

	"kigumi/internal/syntax"
)

func (c *checker) hasNamedArg(args []syntax.NodeID) bool {
	for _, a := range args {
		if c.t.Kind(a) == syntax.NamedArg {
			return true
		}
	}
	return false
}

// paramNames covers declParams[:fixed]; the variadic tail stays unnamed.
func (c *checker) paramNames(declParams []EntityID, fixed int) []string {
	n := min(fixed, len(declParams))
	names := make([]string, n)
	for i := 0; i < n; i++ {
		names[i] = c.r.Entities[declParams[i]].Name
	}
	return names
}

// A name missing from the call always double-fills another slot first, so
// it surfaces as a dup or unknown rather than needing its own case here.
type namedLayout struct {
	order    []syntax.NodeID
	tail     []syntax.NodeID
	badOrder []syntax.NodeID
	unknown  []syntax.NodeID
	dup      []syntax.NodeID
}

func (lay namedLayout) ok() bool {
	return len(lay.badOrder) == 0 && len(lay.unknown) == 0 && len(lay.dup) == 0
}

func (lay namedLayout) args() []syntax.NodeID {
	return append(append([]syntax.NodeID{}, lay.order...), lay.tail...)
}

// Reports no diagnostics itself, so it doubles as a silent viability probe
// for overload candidates.
func (c *checker) layoutNamed(args []syntax.NodeID, names []string) namedLayout {
	lay := namedLayout{order: make([]syntax.NodeID, len(names))}
	filled := make([]bool, len(names))
	seenNamed, pos := false, 0
	for _, a := range args {
		if c.t.Kind(a) != syntax.NamedArg {
			if seenNamed {
				lay.badOrder = append(lay.badOrder, a)
			} else if pos < len(names) {
				filled[pos] = true
				lay.order[pos] = a
			} else {
				lay.tail = append(lay.tail, a)
			}
			pos++
			continue
		}
		seenNamed = true
		name := c.t.TokText(c.t.Nodes[a].Tok)
		switch idx := slices.Index(names, name); {
		case idx < 0:
			lay.unknown = append(lay.unknown, a)
		case filled[idx]:
			lay.dup = append(lay.dup, a)
		default:
			filled[idx] = true
			lay.order[idx] = syntax.NodeID(c.t.Nodes[a].Lhs)
		}
	}
	return lay
}

// declParams == nil means the callee has no parameter names (a fn-typed
// value): named args are rejected and stripped for best-effort positional checking.
func (c *checker) resolveNamedArgs(fnName string, args []syntax.NodeID, declParams []EntityID, fixed int) ([]syntax.NodeID, bool) {
	if declParams == nil {
		for _, a := range args {
			if c.t.Kind(a) == syntax.NamedArg {
				c.errAt(a, cNamedArgOnFnValue)
			}
		}
		return c.unwrapNamed(args), true
	}
	lay := c.layoutNamed(args, c.paramNames(declParams, fixed))
	if !lay.ok() {
		c.reportNamedLayout(fnName, lay)
		return nil, false
	}
	return lay.args(), true
}

func (c *checker) unwrapNamed(args []syntax.NodeID) []syntax.NodeID {
	out := make([]syntax.NodeID, len(args))
	for i, a := range args {
		if c.t.Kind(a) == syntax.NamedArg {
			out[i] = syntax.NodeID(c.t.Nodes[a].Lhs)
		} else {
			out[i] = a
		}
	}
	return out
}

func (c *checker) reportNamedLayout(fnName string, lay namedLayout) {
	for _, a := range lay.badOrder {
		c.errAt(a, cPositionalAfterNamed)
	}
	for _, a := range lay.unknown {
		c.errAt(a, cNamedArgUnknown, fnName, c.t.TokText(c.t.Nodes[a].Tok))
	}
	for _, a := range lay.dup {
		c.errAt(a, cNamedArgDuplicate, c.t.TokText(c.t.Nodes[a].Tok))
	}
}
