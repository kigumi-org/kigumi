package syntax_test

import (
	"testing"

	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func TestDecode(t *testing.T) {
	if got := syntax.DecodeString(`a\tb\u{1F600}\"$`); got != "a\tb😀\"$" {
		t.Errorf("DecodeString = %q", got)
	}
	if r, ok := syntax.DecodeChar(`'\n'`); !ok || r != '\n' {
		t.Errorf("DecodeChar = %q %v", r, ok)
	}
	if _, ok := syntax.DecodeChar(`'ab'`); ok {
		t.Error("DecodeChar accepted two characters")
	}
	if v, ok := syntax.ParseInt("0x_ff"); !ok || v.Int64() != 255 {
		t.Errorf("ParseInt(0x_ff) = %v %v", v, ok)
	}
	if v, ok := syntax.ParseInt("1_000"); !ok || v.Int64() != 1000 {
		t.Errorf("ParseInt(1_000) = %v %v", v, ok)
	}
	if v, ok := syntax.ParseFloat("1.5e-3"); !ok || v.String() != "0.0015" {
		t.Errorf("ParseFloat = %v %v", v, ok)
	}
	if got := syntax.DecodeByteString(`b"\x30\x82A\r\n\t\\\"\0"`); string(got) != "\x30\x82A\r\n\t\\\"\x00" {
		t.Errorf("DecodeByteString = %q", got)
	}
	if got := syntax.DecodeByteString(`b""`); len(got) != 0 {
		t.Errorf("DecodeByteString(empty) = %q, want empty", got)
	}
}

func TestStringParts(t *testing.T) {
	src := `let s = "a ${b} c ${d.e()}"`
	tree := syntax.Parse(token.NewFile("x", []byte(src)))
	lit := syntax.NodeID(tree.Slot(tree.Children(tree.Root)[0], "init"))
	parts := tree.StringParts(lit)
	var got []string
	for _, p := range parts {
		if p.Expr != 0 {
			got = append(got, "<"+tree.Kind(p.Expr).String()+">")
		} else {
			got = append(got, string(tree.File.Src[p.Text.Start:p.Text.End]))
		}
	}
	want := []string{"a ", "<Ident>", " c ", "<Call>"}
	if len(got) != len(want) {
		t.Fatalf("parts = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("part %d = %q, want %q", i, got[i], want[i])
		}
	}
	if !tree.HasTopLevelStatements() || tree.HasErrors() {
		t.Error("HasTopLevelStatements/HasErrors wrong")
	}
}
