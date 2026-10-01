package sem

import "testing"

// mirrors internal/driver's E994 template (HeaderImportSkipped) rather than a hand-duplicated string.
func TestFillTemplate(t *testing.T) {
	got := FillTemplate("importing `{path}` skipped {n} declaration{s}: {list}", "h.h", 3, Plural(3), "a; b; c")
	want := "importing `h.h` skipped 3 declaration" + "s" + ": a; b; c"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if Plural(1) != "" {
		t.Errorf("Plural(1) = %q, want \"\"", Plural(1))
	}
	if Plural(2) != "s" {
		t.Errorf("Plural(2) = %q, want \"s\"", Plural(2))
	}
}
