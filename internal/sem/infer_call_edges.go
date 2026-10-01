package sem

import (
	"slices"

	"kigumi/internal/syntax"
)

// Each fn-typed arg contributes its own effects; a named edge only counts
// once the callee body is known to call that parameter.
func (c *checker) callbackEdges(n syntax.NodeID, fn EntityID, sig TypeID, args []syntax.NodeID) {
	tt := c.r.Types
	params := tt.Node(sig).Args
	info := c.r.Fn(fn)
	for i, a := range args {
		if i >= len(params) || tt.Kind(c.vars.resolve(params[i])) != KFn {
			continue
		}
		var param EntityID
		if info.Body != 0 && i < len(info.Params) {
			param = info.Params[i]
		}
		at := c.vars.resolve(c.info.Types[a])
		switch {
		case c.info.Uses[a] != 0 && c.r.Entities[c.info.Uses[a]].Kind == EntFn:
			c.edge(EffectEdge{Kind: EdgeCallback, Target: c.info.Uses[a], Param: param, Node: a})
		case tt.Kind(at) == KClosure:
			c.edge(EffectEdge{Kind: EdgeCallback, Target: tt.Node(at).Ent, Param: param, Node: a})
		case tt.Kind(at) == KFn:
			c.callbackEdge(a, Effects(tt.Node(at).Flags), param)
		}
	}
}

// When the value is a parameter of the enclosing function, that function
// becomes polymorphic in it instead of being blamed for its effects.
func (c *checker) callbackEdge(expr syntax.NodeID, mods Effects, param EntityID) {
	if p := c.polyParam(expr); p != 0 {
		c.deferToParam(p)
		c.edge(EffectEdge{Kind: EdgeParamCall, Target: p, Node: expr})
		return
	}
	c.edge(EffectEdge{Kind: EdgeCallback, Mods: mods, Param: param, Node: expr})
}

// A non-fn-typed p (from a record-typed parameter's field fact) has
// no matching argument slot in callbackEdges, so it's skipped here.
func (c *checker) deferToParam(p EntityID) {
	if c.r.Types.Kind(c.r.Entities[p].Type) != KFn {
		return
	}
	info := c.r.Fn(c.r.Entities[p].Parent)
	if !slices.Contains(info.CalledParams, p) {
		info.CalledParams = append(info.CalledParams, p)
	}
}

// polyParam excludes closure parameters, which keep the declared-type rule.
func (c *checker) polyParam(expr syntax.NodeID) EntityID {
	for c.t.Kind(expr) == syntax.CallExpr || c.t.Kind(expr) == syntax.WsCallExpr {
		expr = syntax.NodeID(c.t.Nodes[expr].Lhs)
	}
	p := c.info.Uses[expr]
	if alias, ok := c.paramAlias[p]; ok {
		p = alias
	}
	if p == 0 || c.r.Entities[p].Kind != EntParam || c.r.Entities[c.r.Entities[p].Parent].Kind != EntFn {
		return 0
	}
	return p
}

func (c *checker) abandonCall(sig TypeID, args []syntax.NodeID) TypeID {
	c.synthArgs(args)
	c.vars.poison(sig)
	return TyPoison
}

func (c *checker) variadicForm(args []syntax.NodeID, sig TypeID) VariadicForm {
	if c.r.Types.Node(sig).Flags&fnVariadic == 0 {
		return VariadicNone
	}
	if len(args) > 0 && c.t.Kind(args[len(args)-1]) == syntax.Spread {
		return VariadicSpread
	}
	return VariadicElements
}
