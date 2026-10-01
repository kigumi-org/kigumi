package diag

import (
	"fmt"
	"strconv"
	"strings"

	"kigumi/internal/token"
)

// Render formats one diagnostic the way the CLI prints it and the golden
// tests pin it (the layout rustc made familiar):
//
//	error[E100]: undefined name `foo`
//	 --> main.kg:3:9
//	  |
//	3 | let x = foo
//	  |         ^^^
//	  = help: did you mean `for`?
//
// Notes with a location follow as `note:` blocks with their own snippet.
func Render(src Sources, d Diagnostic) string {
	var sb strings.Builder
	renderInto(&sb, src, d)
	return sb.String()
}

func RenderAll(src Sources, ds []Diagnostic) string {
	var sb strings.Builder
	for _, d := range ds {
		renderInto(&sb, src, d)
	}
	return sb.String()
}

func renderInto(sb *strings.Builder, src Sources, d Diagnostic) {
	f := src.Source(d.Loc.Source)
	width := gutterWidth(src, d)
	renderHead(sb, d)
	if f != nil {
		renderSnippet(sb, f, d.Loc.Span, d.Severity, width)
	}
	for _, h := range d.Help {
		fmt.Fprintf(sb, "%s = %s %s\n", strings.Repeat(" ", width), paint(ansiCyan, "help:"), h)
	}
	for _, n := range d.Notes {
		renderHead(sb, n)
		if nf := src.Source(n.Loc.Source); nf != nil {
			renderSnippet(sb, nf, n.Loc.Span, n.Severity, width)
		}
	}
	sb.WriteByte('\n')
}

func renderHead(sb *strings.Builder, d Diagnostic) {
	word := d.Severity.String()
	if d.Code != "" {
		word += "[" + d.Code + "]"
	}
	fmt.Fprintf(sb, "%s%s\n", paint(severityColor(d.Severity), word), paint(ansiBold, ": "+d.Msg))
}

// renderSnippet prints the location, the source line and the underline.
func renderSnippet(sb *strings.Builder, f *token.File, span token.Span, sev Severity, width int) {
	line, col := f.Line(span.Start), f.Column(span.Start)
	pad := strings.Repeat(" ", width)
	fmt.Fprintf(sb, "%s%s %s:%d:%d\n", pad, paint(ansiBlue, "-->"), f.Name, line, col)
	fmt.Fprintf(sb, "%s %s\n", pad, paint(ansiBlue, "|"))
	num := strconv.Itoa(line)
	fmt.Fprintf(sb, "%s%s %s %s\n", strings.Repeat(" ", width-len(num)), paint(ansiBlue, num), paint(ansiBlue, "|"), f.LineText(line))
	carets := strings.Repeat("^", caretWidth(f, span, line))
	fmt.Fprintf(sb, "%s %s %s%s\n", pad, paint(ansiBlue, "|"), strings.Repeat(" ", col-1), paint(underlineColor(sev), carets))
}

// gutterWidth is the width of the widest line number the diagnostic and
// its notes show, so their gutters line up.
func gutterWidth(src Sources, d Diagnostic) int {
	width := 1
	consider := func(loc Location) {
		if f := src.Source(loc.Source); f != nil {
			width = max(width, len(strconv.Itoa(f.Line(loc.Span.Start))))
		}
	}
	consider(d.Loc)
	for _, n := range d.Notes {
		consider(n.Loc)
	}
	return width
}

func caretWidth(f *token.File, span token.Span, line int) int {
	ls := f.LineSpan(line)
	end := min(span.End, ls.End)
	return max(int(end-span.Start), 1)
}
