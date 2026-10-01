package syntax_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kigumi/internal/diag"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// Fixtures live in testdata/parser/*.txtar with a `-- src --` section and an
// `-- expect --` section holding the tree dump followed by diagnostics.
// UPDATE=1 go test ./internal/syntax rewrites the expectations.
func TestParserCorpus(t *testing.T) {
	files, err := filepath.Glob("../../testdata/parser/*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no fixtures found")
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			src, expect, ok := splitTxtar(string(raw))
			if !ok {
				t.Fatalf("%s: missing -- src -- or -- expect -- section", path)
			}
			got := render(filepath.Base(path), src)
			if os.Getenv("UPDATE") == "1" {
				out := "-- src --\n" + src + "-- expect --\n" + got
				if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			if got != expect {
				t.Errorf("mismatch (run with UPDATE=1 to accept)\n--- got ---\n%s--- want ---\n%s", got, expect)
			}
		})
	}
}

func render(name, src string) string {
	f := token.NewFile(name, []byte(src))
	tree := syntax.Parse(f)
	var sb strings.Builder
	sb.WriteString(tree.Dump())
	if len(tree.Diags) > 0 {
		sb.WriteString("-- diagnostics --\n")
		sb.WriteString(diag.RenderAll(f, tree.Diags))
	}
	return sb.String()
}

func splitTxtar(s string) (src, expect string, ok bool) {
	const srcHdr, expHdr = "-- src --\n", "-- expect --\n"
	i := strings.Index(s, srcHdr)
	j := strings.Index(s, expHdr)
	if i < 0 || j < 0 || j < i {
		return "", "", false
	}
	return s[i+len(srcHdr) : j], s[j+len(expHdr):], true
}

// Drives every mutually-recursive entry point deep enough to hit the nesting limit.
func TestParseDoesNotOverflowTheStack(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"blocks", strings.Repeat("{\n", 4000) + "1\n" + strings.Repeat("}\n", 4000)},
		{"parens", strings.Repeat("(", 6000) + "1" + strings.Repeat(")", 6000) + "\n"},
		{"unary", "print " + strings.Repeat("- ", 11000) + "1\n"},
		{"elseIf", "fn f() -> Unit {\nif true {1}\n" +
			strings.Repeat("else if true {1}\n", 11000) + "else {1}\n}\n"},
		{"typePointers", "fn f(x: " + strings.Repeat("*const ", 11000) + "Int) {}\n"},
		{"patternCtors", "fn f() { match x { " + strings.Repeat("Foo(", 11000) + "1" +
			strings.Repeat(")", 11000) + " -> 1, _ -> 0 } }\n"},
		{"stringInterpolation", `print "` + strings.Repeat(`${"`, 11000) + "1" +
			strings.Repeat(`}"`, 11000) + "\"\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file := token.NewFile("x", []byte(c.src))
			done := make(chan *syntax.Tree, 1)
			go func() { done <- syntax.Parse(file) }()
			select {
			case tree := <-done:
				if tree.Root == 0 {
					t.Fatal("Parse returned no root")
				}
				if !slices.ContainsFunc(tree.Diags, func(d diag.Diagnostic) bool {
					return strings.Contains(d.Msg, "nesting is too deep")
				}) {
					t.Fatalf("expected a nesting-too-deep diagnostic among %d diagnostics", len(tree.Diags))
				}
			// A hang guard, not a performance bound: generous so contention
			// doesn't turn it into a false failure.
			case <-time.After(30 * time.Second):
				t.Fatal("Parse did not terminate")
			}
		})
	}
}

// Guards against parenFollowedByFatArrow regressing to an O(n) rescan per `(`.
func TestParenLambdaDisambiguationIsLinear(t *testing.T) {
	const n = 4500
	src := strings.Repeat("(", n) + "1" + strings.Repeat(")", n) + "\nlet f = (a: Int) => a\n"
	start := time.Now()
	tree := syntax.Parse(token.NewFile("x", []byte(src)))
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("parsing %d nested parens took %s; want well under a second", n, elapsed)
	}
	if len(tree.Diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", tree.Diags)
	}
	if !strings.Contains(tree.Dump(), "Lambda") {
		t.Fatal("lambda after deeply nested parens was not recognized")
	}
}

// dumpTiming parses n nested pointer types and times Tree.Dump alone.
func dumpTiming(t *testing.T, n int) (out string, elapsed time.Duration) {
	t.Helper()
	src := "fn f(x: " + strings.Repeat("*const ", n) + "Int) {}\n"
	tree := syntax.Parse(token.NewFile("x", []byte(src)))
	if len(tree.Diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", tree.Diags)
	}
	start := time.Now()
	out = tree.Dump()
	return out, time.Since(start)
}

// Compares ns/byte at two sizes instead of a fixed wall-clock ceiling, which
// either flakes under load or gets too loose to catch a regression.
func TestDumpDeepNestingStaysFast(t *testing.T) {
	const n = 9000 // type-pointer nesting; under the parser's depth limit
	const baseline = 900
	baseOut, baseElapsed := dumpTiming(t, baseline)
	out, elapsed := dumpTiming(t, n)
	if out == "" {
		t.Fatal("empty dump")
	}
	if baseElapsed < 100*time.Microsecond {
		baseElapsed = 100 * time.Microsecond // floor: avoid timer-noise ratios
	}
	baseThroughput := float64(baseElapsed) / float64(len(baseOut))
	throughput := float64(elapsed) / float64(len(out))
	const slack = 10
	if throughput > baseThroughput*slack {
		t.Fatalf("dumping %d nested pointer types cost %.2f ns/byte vs %.2f ns/byte at n=%d; want roughly flat per-byte cost", n, throughput, baseThroughput, baseline)
	}
}

func TestParseNeverPanics(t *testing.T) {
	inputs := []string{
		"", "\n", "}", ")", "let", "let x", "fn", "fn (", "type X =", "match {",
		"if {", "for in {", "$", "$\"", "\"${\"", "a |>", "|> a", "@", "pub pub",
		"import from", "x = ", "f(", "Name {", "let Some( = 1", "extern(C) {",
	}
	for _, in := range inputs {
		f := token.NewFile("x", []byte(in))
		tree := syntax.Parse(f)
		if tree.Root == 0 {
			t.Errorf("Parse(%q): no root", in)
		}
	}
}
