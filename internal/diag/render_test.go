package diag_test

import (
	"testing"

	"kigumi/internal/diag"
	"kigumi/internal/token"
)

func TestRender(t *testing.T) {
	f := token.NewFile("a.kg", []byte("let x = foo bar baz\nlet y = 1\n"))
	d := diag.Errorf(diag.At(f, token.Span{Start: 12, End: 15}), "whitespace call chain is not allowed").
		WithNote(diag.At(f, token.Span{Start: 8, End: 11}), "callee is here").
		WithHelp("wrap the arguments in parentheses")
	d.Code = "E700"
	want := "error[E700]: whitespace call chain is not allowed\n" +
		" --> a.kg:1:13\n" +
		"  |\n" +
		"1 | let x = foo bar baz\n" +
		"  |             ^^^\n" +
		"  = help: wrap the arguments in parentheses\n" +
		"note: callee is here\n" +
		" --> a.kg:1:9\n" +
		"  |\n" +
		"1 | let x = foo bar baz\n" +
		"  |         ^^^\n" +
		"\n"
	if got := diag.Render(f, d); got != want {
		t.Errorf("Render mismatch\n got: %q\nwant: %q", got, want)
	}
}

// Colors wrap the words a reader scans for and leave the text intact.
func TestRenderColor(t *testing.T) {
	diag.UseColor(true)
	defer diag.UseColor(false)
	f := token.NewFile("a.kg", []byte("let x = 1\n"))
	got := diag.Render(f, diag.Warnf(diag.At(f, token.Span{Start: 4, End: 5}), "unused"))
	if want := "\x1b[1;33mwarning\x1b[0m\x1b[1m: unused\x1b[0m\n"; got[:len(want)] != want {
		t.Errorf("colored header = %q", got[:len(want)])
	}
}

func TestBagSortAndLimit(t *testing.T) {
	f := token.NewFile("a.kg", []byte("abc\n"))
	var b diag.Bag
	b.Add(diag.Errorf(diag.At(f, token.Span{Start: 2, End: 3}), "second"))
	b.Add(diag.Errorf(diag.At(f, token.Span{Start: 0, End: 1}), "first"))
	ds := b.Sorted()
	if len(ds) != 2 || ds[0].Msg != "first" {
		t.Fatalf("sorted = %+v", ds)
	}
}

// A note may point into another file than the diagnostic.
func TestRenderAcrossFiles(t *testing.T) {
	store := &token.SourceStore{}
	a, b := token.NewFile("a.kg", []byte("let x = 1\n")), token.NewFile("b.kg", []byte("let x = 2\n"))
	store.Add(a)
	store.Add(b)
	d := diag.Errorf(diag.At(a, token.Span{Start: 4, End: 5}), "redeclared").WithNote(diag.At(b, token.Span{Start: 4, End: 5}), "first here")
	got := diag.Render(store, d)
	if want := " --> b.kg:1:5\n"; !contains(got, want) {
		t.Errorf("note location missing in %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
