package sem

import "kigumi/internal/syntax"

// ConstArgKind says how a caller produces one hidden const-parameter
// argument.
type ConstArgKind uint8

const (
	ConstArgLit ConstArgKind = iota
	ConstArgLocal
)

// ConstArg is one hidden const-parameter argument a call passes after its
// witness arguments, mirroring WitnessArg's role for interface witnesses.
type ConstArg struct {
	Kind  ConstArgKind
	Value int64
	Type  TypeID
	Local EntityID
}

// constParamsOf must enumerate params in the same order declareConstParams
// binds their hidden locals, since calls match ConstArgs positionally.
func (r *Result) constParamsOf(id EntityID) []EntityID {
	info := r.Fn(id)
	var out []EntityID
	seen := map[EntityID]bool{}
	for _, p := range append(append([]EntityID{}, info.TypeParams...), info.RecvParams...) {
		if seen[p] || !r.typeParam(p).IsConst {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// constParamLocalName stays unique per method even when it reuses its
// owner's type-param entity unchanged, since two methods never share a
// declScope.
func constParamLocalName(r *Result, param EntityID) string {
	return "$const:" + r.Entities[param].Name
}

// declareConstParams stores each const parameter as a hidden local so a
// closure captures it like any other local.
func (r *Result) declareConstParams(id EntityID) {
	info := r.Fn(id)
	scope := r.declScope(id)
	for _, p := range r.constParamsOf(id) {
		pinfo := r.typeParam(p)
		local := r.newEntity(Entity{Kind: EntLocal, Name: r.Entities[p].Name, Pkg: r.Entities[id].Pkg, File: r.Entities[id].File, Node: r.Entities[p].Node, Parent: id, Type: pinfo.ConstType})
		r.Entities[local].Detail = r.addLocal(LocalInfo{Scope: scope})
		r.define(scope, constParamLocalName(r, p), Binding{Ent: local})
		info.ConstParams = append(info.ConstParams, local)
	}
}

// constParamLocal mirrors witnessLocal for interface witnesses.
func (c *checker) constParamLocal(n syntax.NodeID, param EntityID) EntityID {
	b, found, ok := c.r.lookup(c.scope, constParamLocalName(c.r, param))
	if !ok || b.Ent == 0 {
		return 0
	}
	c.noteCapture(n, b.Ent, found)
	return b.Ent
}

// constArgFromType builds the ConstArg for a resolved instantiation
// argument; n and scope locate the call site
// so a forwarded const parameter is captured like a witness argument.
func (c *checker) constArgFromType(n syntax.NodeID, t TypeID) ConstArg {
	tt := c.r.Types
	switch tt.Kind(t) {
	case KConst:
		v, ty := tt.ConstValue(t)
		return ConstArg{Kind: ConstArgLit, Value: v, Type: ty}
	case KParam:
		if info := c.r.typeParam(tt.Node(t).Ent); info != nil && info.IsConst {
			return ConstArg{Kind: ConstArgLocal, Local: c.constParamLocal(n, tt.Node(t).Ent), Type: info.ConstType}
		}
	}
	return ConstArg{}
}

// allowedConstType reports whether t is a legal const parameter type:
// usize, a fixed-width integer, Int or Bool.
func (r *Result) allowedConstType(t TypeID) bool {
	tt := r.Types
	return t == TyBool || tt.IsInteger(t)
}
