package sem

import (
	"strings"

	"kigumi/internal/syntax"
)

// checkMatch decides exhaustiveness and redundancy for a match whose arms
// are already typed; narrowing facts on the scrutinee place add
// synthetic rows.
func (c *checker) checkMatch(n syntax.NodeID, scrutineeNode syntax.NodeID, scrutinee TypeID, arms []syntax.NodeID) {
	scrutinee = c.vars.resolve(scrutinee)
	info := MatchInfo{Exhaustive: true}
	if scrutinee == TyPoison {
		c.info.Matches[n] = info
		return
	}
	types := []TypeID{scrutinee}
	var rows [][]pat
	place := c.placeOf(scrutineeNode)
	for _, f := range c.facts {
		if f.Place != place || place == 0 {
			continue
		}
		for _, v := range c.excludedBy(scrutinee, f) {
			rows = append(rows, []pat{c.variantPat(scrutinee, v)})
			info.AssumedExcluded = append(info.AssumedExcluded, v)
		}
	}
	synthetic := len(rows)
	guarded := false
	for i, arm := range arms {
		s := matchArm(c.t, arm)
		alts := c.abstractAll(s.Pattern)
		if len(alts) == 0 || hasErr(alts[0]) {
			continue
		}
		reachable := false
		for _, alt := range alts {
			reachable = reachable || c.useful(rows[synthetic:], []pat{alt}, types)
		}
		if !reachable {
			c.errAt(s.Pattern, cMatchUnreachable)
			info.Unreachable = append(info.Unreachable, i)
		}
		if s.Guard != 0 {
			guarded = true
			continue
		}
		for _, alt := range alts {
			rows = append(rows, []pat{alt})
		}
	}
	if scrutinee == TyNever {
		c.info.Matches[n] = info
		return
	}
	for len(info.Missing) < 3 {
		w, ok := c.missing(rows, types)
		if ok {
			break
		}
		info.Missing = append(info.Missing, c.renderPat(w[0]))
		rows = append(rows, w)
	}
	if len(info.Missing) > 0 {
		info.Exhaustive = false
		if d, ok := c.r.diagAt(c.f, n, cMatchNonExhaustive, strings.Join(info.Missing, "`, `")); ok {
			if guarded {
				d = d.WithNote(c.loc(n), "arms with a guard do not count")
			}
			c.r.emitDiag(c.f, d)
		}
	}
	c.info.Matches[n] = info
}

// excludedBy lists the variants a fact rules out for the scrutinee.
func (c *checker) excludedBy(scrutinee TypeID, f NarrowFact) []EntityID {
	switch f.Kind {
	case FactIsNot:
		return []EntityID{f.Variant}
	case FactIs:
		var out []EntityID
		for _, v := range c.r.typeDecl(c.r.Entities[f.Variant].Parent).Variants {
			if v != f.Variant {
				out = append(out, v)
			}
		}
		return out
	}
	return nil
}

func (c *checker) variantPat(scrutinee TypeID, v EntityID) pat {
	return pat{kind: pCtor, key: variantKey(v), name: c.r.Entities[v].Name, args: wilds(len(c.r.variant(v).Payload))}
}

// renderPat writes a witness in source form.
func (c *checker) renderPat(p pat) string {
	switch p.kind {
	case pWild:
		return "_"
	case pLit:
		return p.name
	}
	if strings.HasPrefix(p.key, "r") {
		ent := EntityID(atoi(p.key[1:]))
		if c.r.Types.isTupleEnt(ent) {
			parts := make([]string, len(p.args))
			for i, a := range p.args {
				parts[i] = c.renderPat(a)
			}
			return "(" + strings.Join(parts, ", ") + ")"
		}
		var parts []string
		for i, f := range c.r.typeDecl(ent).Fields {
			if i < len(p.args) && p.args[i].kind != pWild {
				parts = append(parts, c.r.Entities[f].Name+": "+c.renderPat(p.args[i]))
			}
		}
		if len(parts) == 0 {
			return "_"
		}
		return p.name + " { " + strings.Join(parts, ", ") + " }"
	}
	if len(p.args) == 0 {
		return p.name
	}
	var parts []string
	for _, a := range p.args {
		parts = append(parts, c.renderPat(a))
	}
	return p.name + "(" + strings.Join(parts, ", ") + ")"
}

func atoi(s string) int {
	n := 0
	for _, ch := range s {
		n = n*10 + int(ch-'0')
	}
	return n
}
