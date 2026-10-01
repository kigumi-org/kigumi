package sem

import "kigumi/internal/syntax"

func (r *Result) checkTestBody(id EntityID) {
	e := &r.Entities[id]
	body := syntax.NodeID(r.tree(e.File).Nodes[e.Node].Rhs)
	scope := r.newScope(ScopeFn, r.fileScopes[e.File], e.File, e.Node)
	r.Scopes[scope].Fn = id
	var edges []EffectEdge
	c := r.newChecker(e.File, id, scope, r.unitResult(), &edges)
	r.Bodies = append(r.Bodies, BodyRef{Fn: id, File: e.File, Node: body})
	c.checkBodyBlock(body)
	r.Fn(id).Edges = edges
}

func (r *Result) unitResult() TypeID { return r.Types.Result(TyUnit, r.Types.errorType()) }
