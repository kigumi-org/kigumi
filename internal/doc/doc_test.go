package doc_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kigumi/internal/doc"
	"kigumi/internal/driver"
)

func TestPackageMarkdown(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "shapes"), 0o755)
	src := "/// A shape with an area.\npub interface Shape {\n    fn area(self) -> Int\n}\n\n/// Axis-aligned square.\npub type Square = {\n    pub side Int\n}\n\n/// Area of the square.\npub fn Square.area(self) -> Int {\n    self.side * self.side\n}\n\nfn hidden() -> Int {\n    1\n}\n\n/// Unit square.\npub const unit = 1\n"
	os.WriteFile(filepath.Join(root, "shapes", "shapes.kg"), []byte(src), 0o644)
	m, err := driver.LoadModule(root, driver.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pages := doc.Module(m, doc.Options{})
	page := pages["shapes"]
	for _, want := range []string{"# shapes", "## Types", "### Square", "```kigumi\npub type Square = {\n    pub side Int\n}\n```", "Axis-aligned square.", "#### Square.area", "pub fn Square.area(self) -> Int", "Area of the square.", "## Interfaces", "### Shape", "## Constants", "### unit"} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q in:\n%s", want, page)
		}
	}
	if strings.Contains(page, "hidden") {
		t.Errorf("private declaration rendered:\n%s", page)
	}
	if all := doc.Module(m, doc.Options{All: true})["shapes"]; !strings.Contains(all, "### hidden") {
		t.Errorf("--all should include private declarations:\n%s", all)
	}
	if idx := doc.Index(pages); !strings.Contains(idx, "[shapes](shapes.md)") {
		t.Errorf("index: %q", idx)
	}
}
