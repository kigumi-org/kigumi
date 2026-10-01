package token_test

import (
	"testing"

	"kigumi/internal/token"
)

func TestLookup(t *testing.T) {
	tests := []struct {
		in   string
		want token.Kind
	}{
		{"fn", token.KwFn},
		{"nil", token.Ident},
		{"static", token.Ident},
		{"impl", token.Ident},
		{"allocator", token.Ident},
		{"with", token.Ident},
		{"weak", token.Ident},
	}
	for _, tt := range tests {
		if got := token.Lookup(tt.in); got != tt.want {
			t.Errorf("Lookup(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestKindString(t *testing.T) {
	if token.PipeGt.String() != "|>" {
		t.Errorf("PipeGt.String() = %q", token.PipeGt.String())
	}
	if token.KwErrdefer.String() != "errdefer" {
		t.Errorf("KwErrdefer.String() = %q", token.KwErrdefer.String())
	}
	if !token.KwFalse.IsKeyword() || token.Ident.IsKeyword() {
		t.Error("IsKeyword boundary is wrong")
	}
}

func TestFileLineColumn(t *testing.T) {
	f := token.NewFile("x", []byte("ab\ncd\n\nef"))
	tests := []struct {
		pos       token.Pos
		line, col int
	}{
		{0, 1, 1}, {1, 1, 2}, {2, 1, 3}, {3, 2, 1}, {4, 2, 2}, {6, 3, 1}, {7, 4, 1}, {8, 4, 2},
	}
	for _, tt := range tests {
		if l, c := f.Line(tt.pos), f.Column(tt.pos); l != tt.line || c != tt.col {
			t.Errorf("pos %d: got %d:%d, want %d:%d", tt.pos, l, c, tt.line, tt.col)
		}
	}
	if got := f.LineText(2); got != "cd" {
		t.Errorf("LineText(2) = %q", got)
	}
	if got := f.LineText(4); got != "ef" {
		t.Errorf("LineText(4) = %q", got)
	}
	if f.LineCount() != 4 {
		t.Errorf("LineCount = %d", f.LineCount())
	}
}
