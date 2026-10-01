package mir

import (
	"kigumi/internal/diag"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

type owner struct {
	p        *Program
	f        *Func
	pl       *places
	tracked  map[LocalID]bool
	in       []*dstate
	reported map[syntax.NodeID]bool
	defNode  map[LocalID]syntax.NodeID
}

func cloneState(s map[LocalID]ownState) map[LocalID]ownState {
	out := make(map[LocalID]ownState, len(s))
	for k, v := range s {
		out[k] = v
	}
	return out
}

// merge joins a predecessor's exit state into a block; it reports
// whether the block's entry state changed.
func (a *owner) merge(bid BlockID, out *dstate) bool {
	cur := a.in[bid]
	if cur == nil {
		a.in[bid] = cloneD(out)
		return true
	}
	c1 := mergeMap(cur.val, out.val)
	c2 := mergeMap(cur.field, out.field)
	return c1 || c2
}

// mergeMap joins one dstate map's predecessor exit into a block's entry.
func mergeMap(cur, out map[LocalID]ownState) bool {
	changed := false
	for l, s := range out {
		if c, ok := cur[l]; !ok {
			cur[l] = s
			changed = true
		} else if j := joinOwn(c, s); j != c {
			cur[l] = j
			changed = true
		}
	}
	return changed
}

func (a *owner) check(in *Inst, l LocalID, st map[LocalID]ownState, report bool) {
	if !report || in == nil {
		return
	}
	name := a.f.Locals[l].Name
	if n, ok := a.pl.name[l]; ok {
		name = n
	}
	switch st[l] {
	case ownMoved:
		a.report(in.Node, "use-after-move", "`"+name+"` was moved; it cannot be used again")
	case ownConflict:
		a.report(in.Node, "move-join-mismatch", "`"+name+"` is moved on some paths but not others")
	}
}

// checkWhole reports a use of l as a whole value while one of its own
// fields is still moved out (E1025), naming that field.
func (a *owner) checkWhole(n syntax.NodeID, l LocalID, st *dstate, report bool) {
	if !report {
		return
	}
	fields := a.movedFields(l, st)
	if len(fields) == 0 {
		return
	}
	code, msg := partialMoveDiag(a.fieldName(l, fields[0]), a.ownerName(l))
	a.report(n, code, msg)
}

// partialMoveDiag renders E1025 from the sem catalog (codes_body.go's
// cMovePartialUse) so own.go and own_partial.go share one wording instead
// of two hand-written literals.
func partialMoveDiag(field, owner string) (string, string) {
	c, _ := sem.CodeByName("move-partial-use")
	msg := sem.FillTemplate(c.Template, field, owner)
	if c.Help != "" {
		msg += "; " + c.Help
	}
	return c.Num, msg
}

// fieldName renders root's field idx the way it was written at its move.
func (a *owner) fieldName(root LocalID, idx int) string {
	rep := a.pl.children[root][idx]
	if n, ok := a.pl.name[rep]; ok {
		return n
	}
	return a.f.Locals[rep].Name
}

func (a *owner) ownerName(l LocalID) string {
	if n, ok := a.pl.name[l]; ok {
		return n
	}
	return a.f.Locals[l].Name
}

func (a *owner) report(n syntax.NodeID, code, msg string) {
	if n == 0 || a.reported[n] {
		return
	}
	a.reported[n] = true
	t := a.p.R.Tree(a.f.File)
	a.f.Diags = append(a.f.Diags, diag.Diagnostic{Severity: diag.Error, Loc: diag.At(t.File, t.Span(n)), Msg: msg, Code: code})
}

// HasErrors reports whether any function carries an ownership error.
func (p *Program) HasErrors() bool {
	for _, f := range p.Funcs {
		if len(f.Diags) > 0 {
			return true
		}
	}
	return false
}

// Render prints the ownership diagnostics of every function.
func (p *Program) Render() string {
	out := ""
	for _, f := range p.Funcs {
		if len(f.Diags) == 0 {
			continue
		}
		out += diag.RenderAll(p.R.Tree(f.File).File, f.Diags)
	}
	return out
}
