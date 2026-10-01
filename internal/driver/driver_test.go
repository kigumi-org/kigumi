package driver_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"kigumi/internal/driver"
)

// Every .kg file under examples/ must parse without diagnostics, format
// idempotently, and parse again to the same tree.
func TestExamples(t *testing.T) {
	t.Parallel()
	var files []string
	err := filepath.WalkDir("../../examples", func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(path) == ".kg" {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no examples found")
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := driver.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			tree, rendered := driver.Parse(src)
			if rendered != "" {
				t.Fatalf("diagnostics:\n%s", rendered)
			}
			once, err := driver.Format(src)
			if err != nil {
				t.Fatal(err)
			}
			again := driver.Source{Path: path, Src: []byte(once)}
			tree2, rendered := driver.Parse(again)
			if rendered != "" {
				t.Fatalf("formatted output has diagnostics:\n%s\n%s", once, rendered)
			}
			if normalizeImports(tree2.Dump()) != normalizeImports(tree.Dump()) {
				t.Errorf("parse(fmt(s)) != parse(s)\n%s", once)
			}
			twice, err := driver.Format(again)
			if err != nil {
				t.Fatal(err)
			}
			if twice != once {
				t.Errorf("fmt not idempotent\n--- once ---\n%s--- twice ---\n%s", once, twice)
			}
			if os.Getenv("SHOW") == "1" {
				t.Logf("\n%s", once)
			}
		})
	}
}

func TestLoadModule(t *testing.T) {
	t.Parallel()
	m, err := driver.LoadModule("../../examples/repostat", driver.LoadOptions{Test: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"repostat/cmd/repostat", "repostat/notify", "repostat/parse", "repostat/report"}
	if len(m.Order) != len(want) {
		t.Fatalf("packages = %v, want %v", m.Order, want)
	}
	for i, p := range want {
		if m.Order[i] != p {
			t.Errorf("package %d = %q, want %q", i, m.Order[i], p)
		}
	}
	if n := len(m.Packages["repostat/parse"].Files); n != 2 {
		t.Errorf("repostat/parse has %d files with tests, want 2", n)
	}
	if d := m.Diagnostics(); d != "" {
		t.Errorf("unexpected diagnostics:\n%s", d)
	}
	noTest, err := driver.LoadModule("../../examples/repostat", driver.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(noTest.Packages["repostat/parse"].Files); n != 1 {
		t.Errorf("repostat/parse has %d files without tests, want 1", n)
	}
}

// normalizeImports makes a dump insensitive to the order of a run of
// top-level imports and of the names in one, which the formatter sorts.
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
