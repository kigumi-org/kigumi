package sem

import (
	"cmp"
	"slices"
	"strings"

	"kigumi/internal/diag"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// Code is one diagnostic kind: a stable number, catalog name, and message template.
type Code struct {
	Num      string
	Name     string
	Template string
	Help     string
}

var codeByName = map[string]Code{}
var codes []Code

func defineCode(num, name, template string) Code {
	c := Code{Num: num, Name: name, Template: template}
	if i := strings.Index(template, "; "); i >= 0 {
		c.Template, c.Help = template[:i], template[i+2:]
	}
	codeByName[name] = c
	codes = append(codes, c)
	return c
}

// Codes lists the catalog in definition order, for `kigumi explain`.
func Codes() []Code { return append([]Code{}, codes...) }

// CodeByName looks a code up by its catalog name; tests use it.
func CodeByName(name string) (Code, bool) {
	c, ok := codeByName[name]
	return c, ok
}

// diagKey dedups one code at one position, but only for the same message:
// two subjects reported at one node are two diagnostics.
type diagKey struct {
	file  FileID
	start token.Pos
	code  string
	msg   string
}

func (r *Result) loc(f FileID, span token.Span) diag.Location {
	return diag.At(r.Files[f].Tree.File, span)
}

func (r *Result) removeLine(f FileID, n syntax.NodeID, title string) diag.Fix {
	t := r.Files[f].Tree
	sp := t.Span(n)
	end := t.File.LineSpan(t.File.Line(sp.Start)).End
	if int(end) < len(t.File.Src) && t.File.Src[end] == '\n' {
		end++
	}
	return diag.Fix{Title: title, Loc: diag.At(t.File, token.Span{Start: t.File.LineSpan(t.File.Line(sp.Start)).Start, End: end})}
}

func (r *Result) insertAt(f FileID, tok uint32, title, text string) diag.Fix {
	start := r.Files[f].Tree.Toks[tok].Span.Start
	return diag.Fix{Title: title, Loc: diag.At(r.Files[f].Tree.File, token.Span{Start: start, End: start}), NewText: text}
}

func (r *Result) errTok(f FileID, tok uint32, c Code, args ...any) bool {
	if f == 0 || r.suppressed(args) {
		return false
	}
	d, ok := r.diagSpan(f, r.Files[f].Tree.Toks[tok].Span, c, args...)
	if ok {
		r.emitDiag(f, d)
	}
	return ok
}

// notes carry no code and are never suppressed on their own.
func (r *Result) noteText(template string, args ...any) string { return r.fill(template, args) }

func (r *Result) emit() {
	for i := range r.Files {
		slices.SortStableFunc(r.Files[i].Diags, func(a, b diag.Diagnostic) int {
			return cmp.Or(cmp.Compare(a.Loc.Span.Start, b.Loc.Span.Start), cmp.Compare(a.Severity, b.Severity))
		})
	}
}
