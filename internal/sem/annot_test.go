package sem_test

import (
	"strings"
	"testing"

	"kigumi/internal/diag"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// annotation is one `//~ [locator] KIND text` marker.
type annotation struct {
	file string
	line int
	kind string
	text string
}

func scanAnnotations(file, src string) []annotation {
	var out []annotation
	prevTarget := 0
	for i, l := range strings.Split(src, "\n") {
		idx := strings.Index(l, "//~")
		if idx < 0 {
			continue
		}
		for _, chunk := range strings.Split(l[idx+3:], "//~") {
			out = append(out, parseAnnotation(file, i+1, chunk, &prevTarget))
		}
	}
	return out
}

func parseAnnotation(file string, line int, chunk string, prevTarget *int) annotation {
	{
		rest := strings.TrimSpace(chunk)
		target := line
		switch {
		case strings.HasPrefix(rest, "^"):
			n := len(rest) - len(strings.TrimLeft(rest, "^"))
			target -= n
			rest = strings.TrimSpace(rest[n:])
		case strings.HasPrefix(rest, "v"):
			n := len(rest) - len(strings.TrimLeft(rest, "v"))
			target += n
			rest = strings.TrimSpace(rest[n:])
		case strings.HasPrefix(rest, "|"):
			target = *prevTarget
			rest = strings.TrimSpace(rest[1:])
		}
		kind, text, _ := strings.Cut(rest, " ")
		*prevTarget = target
		return annotation{file: file, line: target, kind: kind, text: strings.TrimSpace(text)}
	}
}

func severityOf(kind string) (diag.Severity, bool) {
	switch kind {
	case "ERROR":
		return diag.Error, true
	case "WARNING":
		return diag.Warning, true
	}
	return 0, false
}

func checkAnnotations(t *testing.T, res *sem.Result, tree *syntax.Tree, src string) {
	f := tree.File
	diags := res.Diagnostics(tree)
	consumed := make([]bool, len(diags))
	for _, a := range scanAnnotations(f.Name, src) {
		switch a.kind {
		case "ERROR", "WARNING":
			want, _ := severityOf(a.kind)
			found := false
			for i, d := range diags {
				if consumed[i] || d.Severity != want || f.Line(d.Loc.Span.Start) != a.line || !strings.Contains(d.Msg, a.text) {
					continue
				}
				consumed[i] = true
				found = true
				break
			}
			if !found {
				t.Errorf("%s:%d: expected %s containing %q, not reported", f.Name, a.line, a.kind, a.text)
			}
		case "NOTE":
			found := false
			for _, d := range diags {
				for _, n := range d.Notes {
					if nf := res.Source(n.Loc.Source); nf != nil && nf != f {
						continue
					}
					if f.Line(n.Loc.Span.Start) == a.line && strings.Contains(n.Msg, a.text) {
						found = true
					}
				}
			}
			if !found {
				t.Errorf("%s:%d: expected note containing %q", f.Name, a.line, a.text)
			}
		case "TYPE":
			checkTypeAnnotation(t, res, tree, a)
		default:
			t.Errorf("%s:%d: unknown annotation kind %q", f.Name, a.line, a.kind)
		}
	}
	for i, d := range diags {
		if !consumed[i] && (d.Severity == diag.Error || d.Severity == diag.Warning) {
			t.Errorf("%s:%d: unexpected %s: %s", f.Name, f.Line(d.Loc.Span.Start), d.Severity, d.Msg)
		}
	}
}

func checkTypeAnnotation(t *testing.T, res *sem.Result, tree *syntax.Tree, a annotation) {
	name, want, ok := strings.Cut(a.text, ":")
	if !ok {
		t.Errorf("%s:%d: TYPE annotation needs `name : Type`", a.file, a.line)
		return
	}
	name, want = strings.TrimSpace(name), strings.TrimSpace(want)
	found := false
	for id := 1; id < len(tree.Nodes); id++ {
		n := tree.Nodes[id]
		switch n.Kind {
		case syntax.Ident, syntax.PatBind, syntax.Param:
		default:
			continue
		}
		if tree.TokText(n.Tok) != name || tree.File.Line(tree.Toks[n.Tok].Start) != a.line {
			continue
		}
		found = true
		if got := res.TypeString(res.TypeOf(tree, syntax.NodeID(id))); got != want {
			t.Errorf("%s:%d: `%s` has type %s, want %s", a.file, a.line, name, got, want)
		}
	}
	if !found {
		t.Errorf("%s:%d: no node named `%s` on this line", a.file, a.line, name)
	}
}
