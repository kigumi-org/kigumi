package sem

import "kigumi/internal/syntax"

// checkImplicitMain checks the entry file's top-level statements as the body
// of `fn main() -> Unit!`, in a script scope under the entry scope.
func (r *Result) checkImplicitMain(id EntityID) {
	e := &r.Entities[id]
	f := e.File
	t := r.tree(f)
	scope := r.newScope(ScopeScript, r.fileScopes[f], f, t.Root)
	r.Scopes[scope].Fn = id
	// The root node names the scope a position outside any body sees.
	r.Files[f].Scopes[t.Root] = scope
	var edges []EffectEdge
	c := r.newChecker(f, id, scope, r.unitResult(), &edges)
	r.Bodies = append(r.Bodies, BodyRef{Fn: id, File: f, Node: t.Root})
	for _, d := range r.topDecls(f) {
		if isStatementKind(t.Kind(d)) {
			c.stmt(d)
		}
	}
	defer func() { r.Fn(id).Edges = edges }()
}

// currentContract keeps the S3 contract explicit: effect edges are
// recorded but not yet evaluated.
func (c *checker) currentContract() Effects {
	if len(c.contracts) > 0 {
		return c.contracts[len(c.contracts)-1]
	}
	if c.fn != 0 && c.r.Entities[c.fn].Kind == EntFn {
		return c.r.Fn(c.fn).Declared
	}
	return 0
}

var _ = (*checker).currentContract

func isEntryNode(t *syntax.Tree, n syntax.NodeID) bool { return n == t.Root }
