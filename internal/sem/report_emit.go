package sem

import (
	"fmt"
	"strings"

	"kigumi/internal/diag"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func (r *Result) noteDecl(d diag.Diagnostic, b Binding, msg string) diag.Diagnostic {
	ent := b.Ent
	if ent == 0 && b.Set != 0 && len(r.Overloads[b.Set].Members) > 0 {
		ent = r.Overloads[b.Set].Members[0]
	}
	if ent == 0 {
		return d
	}
	e := r.Entity(ent)
	if e.File == 0 || e.Tok == 0 {
		return d
	}
	return d.WithNote(r.loc(e.File, r.Files[e.File].Tree.Toks[e.Tok].Span), msg)
}

func (r *Result) errNote(f FileID, n syntax.NodeID, prev Binding, note string, c Code, args ...any) bool {
	d, ok := r.diagAt(f, n, c, args...)
	if ok {
		r.emitDiag(f, r.noteDecl(d, prev, note))
	}
	return ok
}

// a poison argument means the error was already reported at its origin.
func (r *Result) suppressed(args []any) bool {
	for _, a := range args {
		switch v := a.(type) {
		case TypeID:
			if v == TyPoison || v == NoType {
				return true
			}
		case EntityID:
			if r.poisoned(v) {
				return true
			}
		}
	}
	return false
}

func (r *Result) fmtArg(a any) string {
	switch v := a.(type) {
	case TypeID:
		return r.TypeString(v)
	case EntityID:
		return r.entityName(v)
	case string:
		return v
	}
	return fmt.Sprint(a)
}

func (r *Result) fill(template string, args []any) string {
	return fillTemplate(template, r.fmtArg, args)
}

func fillTemplate(template string, fmtArg func(any) string, args []any) string {
	var sb strings.Builder
	rest := template
	for _, a := range args {
		i := strings.IndexByte(rest, '{')
		j := strings.IndexByte(rest, '}')
		if i < 0 || j < i {
			break
		}
		sb.WriteString(rest[:i])
		sb.WriteString(fmtArg(a))
		rest = rest[j+1:]
	}
	sb.WriteString(rest)
	return sb.String()
}

// FillTemplate is fillTemplate for a caller with no *Result (internal/driver builds E994's
// message before a sem.Result exists); args must be a string or an int.
func FillTemplate(template string, args ...any) string {
	return fillTemplate(template, func(a any) string { return fmt.Sprint(a) }, args)
}

// Plural is plural, exported for the same reason as FillTemplate.
func Plural(n int) string { return plural(n) }

func (r *Result) diagAt(f FileID, n syntax.NodeID, c Code, args ...any) (diag.Diagnostic, bool) {
	if f == 0 || r.suppressed(args) {
		return diag.Diagnostic{}, false
	}
	return r.diagSpan(f, r.Files[f].span(n), c, args...)
}

func (r *Result) diagSpan(f FileID, span token.Span, c Code, args ...any) (diag.Diagnostic, bool) {
	// The holes are numbered across message and help, so the help takes
	// the arguments the message did not use.
	used := strings.Count(c.Template, "{")
	msg := r.fill(c.Template, args)
	key := diagKey{f, span.Start, c.Name, msg}
	if _, dup := r.seen[key]; dup {
		return diag.Diagnostic{}, false
	}
	r.seen[key] = struct{}{}
	d := diag.Diagnostic{Severity: diag.Error, Loc: r.loc(f, span), Msg: msg, Code: c.Num}
	if c.Help != "" {
		d.Help = append(d.Help, r.fill(c.Help, args[min(used, len(args)):]))
	}
	if strings.HasPrefix(c.Num, "W") {
		d.Severity = diag.Warning
	}
	return d, true
}

func (r *Result) emitDiag(f FileID, d diag.Diagnostic) {
	r.Files[f].Diags = append(r.Files[f].Diags, d)
}

func (r *Result) errAt(f FileID, n syntax.NodeID, c Code, args ...any) bool {
	d, ok := r.diagAt(f, n, c, args...)
	if ok {
		r.emitDiag(f, d)
	}
	return ok
}

func (r *Result) errFix(f FileID, n syntax.NodeID, fix diag.Fix, c Code, args ...any) bool {
	d, ok := r.diagAt(f, n, c, args...)
	if ok {
		d.Fixes = append(d.Fixes, fix)
		r.emitDiag(f, d)
	}
	return ok
}
