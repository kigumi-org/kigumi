package printer_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"kigumi/internal/diag"
	"kigumi/internal/printer"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// printTiming parses n nested blocks and times printer.Print alone.
func printTiming(t *testing.T, n int) (out string, elapsed time.Duration) {
	t.Helper()
	src := "fn f() {\n" + strings.Repeat("{\n", n) + "1\n" + strings.Repeat("}\n", n) + "}\n"
	tree := syntax.Parse(token.NewFile("x", []byte(src)))
	for _, d := range tree.Diags {
		if d.Severity == diag.Error {
			t.Fatalf("fixture must parse without errors: %v", d)
		}
	}
	start := time.Now()
	out = printer.Print(tree)
	return out, time.Since(start)
}

// TestPrintDeepNestingStaysFast compares ns/byte at two sizes (output is
// quadratic in n) instead of a fixed wall-clock ceiling, which either flakes
// under load or gets too loose to catch a regression.
func TestPrintDeepNestingStaysFast(t *testing.T) {
	const n = 3000 // block nesting; under the parser's depth limit
	const baseline = 300
	baseOut, baseElapsed := printTiming(t, baseline)
	out, elapsed := printTiming(t, n)
	if out == "" {
		t.Fatal("empty output")
	}
	if baseElapsed < 100*time.Microsecond {
		baseElapsed = 100 * time.Microsecond // floor: avoid timer-noise ratios
	}
	baseThroughput := float64(baseElapsed) / float64(len(baseOut))
	throughput := float64(elapsed) / float64(len(out))
	const slack = 10
	if throughput > baseThroughput*slack {
		t.Fatalf("printing %d nested blocks cost %.2f ns/byte vs %.2f ns/byte at n=%d; want roughly flat per-byte cost", n, throughput, baseThroughput, baseline)
	}
}

// TestIdempotent checks fmt(fmt(s)) == fmt(s) and parse(fmt(s)) == parse(s)
// via tree dumps; fixtures under testdata/fmt also pin canonical output.
func TestIdempotent(t *testing.T) {
	files, err := filepath.Glob("../../testdata/parser/*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src := readSrc(t, path)
			tree := syntax.Parse(token.NewFile("a", []byte(src)))
			for _, d := range tree.Diags {
				if d.Severity == diag.Error {
					t.Skip("fixture has errors")
				}
			}
			once := printer.Print(tree)
			again := syntax.Parse(token.NewFile("b", []byte(once)))
			if len(again.Diags) > 0 {
				t.Fatalf("formatted output does not parse:\n%s\n%v", once, again.Diags[0])
			}
			if got, want := normalizeImports(stripNoise(again.Dump())), normalizeImports(stripNoise(tree.Dump())); got != want {
				t.Errorf("parse(fmt(s)) != parse(s)\n--- fmt ---\n%s\n--- got ---\n%s--- want ---\n%s", once, got, want)
			}
			if twice := printer.Print(again); twice != once {
				t.Errorf("fmt is not idempotent\n--- once ---\n%s--- twice ---\n%s", once, twice)
			}
		})
	}
}

// normalizeImports makes a tree dump insensitive to the order of a run of
// top-level imports and of the names inside one, which the formatter sorts
// without changing any binding.
func normalizeImports(dump string) string {
	lines := strings.Split(dump, "\n")
	var out, blocks []string
	flush := func() {
		sort.Strings(blocks)
		out = append(out, blocks...)
		blocks = nil
	}
	for i := 0; i < len(lines); {
		if strings.HasPrefix(lines[i], "  ImportDecl") {
			j := i + 1
			for j < len(lines) && strings.HasPrefix(lines[j], "    ") {
				j++
			}
			blk := append([]string{}, lines[i:j]...)
			sort.Strings(blk[1:])
			blocks = append(blocks, strings.Join(blk, "\n"))
			i = j
			continue
		}
		if blocks != nil {
			flush()
		}
		out = append(out, lines[i])
		i++
	}
	if blocks != nil {
		flush()
	}
	return strings.Join(out, "\n")
}

func TestFmtCorpus(t *testing.T) {
	files, err := filepath.Glob("../../testdata/fmt/*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			i := strings.Index(string(raw), "-- expect --\n")
			src := strings.TrimPrefix(string(raw[:i]), "-- src --\n")
			tree := syntax.Parse(token.NewFile("a", []byte(src)))
			for _, d := range tree.Diags {
				if d.Severity == diag.Error {
					t.Fatalf("fixture must parse without errors: %v", d)
				}
			}
			got := printer.Print(tree)
			if os.Getenv("UPDATE") == "1" {
				os.WriteFile(path, []byte("-- src --\n"+src+"-- expect --\n"+got), 0o644)
				return
			}
			if want := string(raw[i+len("-- expect --\n"):]); got != want {
				t.Errorf("mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
			}
		})
	}
}

func readSrc(t *testing.T, path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	i := strings.Index(s, "-- expect --\n")
	return strings.TrimPrefix(s[:i], "-- src --\n")
}

// stripNoise maps a dump to the normalized AST: whitespace calls and
// ordinary calls are the same call, and `pub(self)` is the same as no
// visibility.
func stripNoise(dump string) string {
	r := strings.NewReplacer("WsCall\n", "Call\n", "vis: Visibility self\n", "vis: -\n")
	return r.Replace(dump)
}
