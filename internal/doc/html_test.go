package doc_test

import (
	"strings"
	"testing"

	"kigumi/internal/doc"
)

// The HTML site has an index, one page per package, and the Markdown
// forms the renderer emits turned into their HTML.
func TestHTMLSite(t *testing.T) {
	pages := map[string]string{"shapes": "# shapes\n\n## Types\n\n### Square\n\n```kigumi\npub type Square = {\n    pub side Int\n}\n```\n\nAxis-aligned `square` with <side>.\n"}
	site := doc.HTML(pages, "demo")
	index, page := site["index.html"], site["shapes.html"]
	if !strings.Contains(index, `<a href="shapes.html">shapes</a>`) || !strings.Contains(index, "<title>demo</title>") {
		t.Errorf("index:\n%s", index)
	}
	for _, want := range []string{`<h2 id="types">Types</h2>`, "<pre><code>pub type Square = {\n    pub side Int\n}\n</code></pre>", "<p>Axis-aligned <code>square</code> with &lt;side&gt;.</p>", `<nav><a href="index.html">Packages</a>`} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q in:\n%s", want, page)
		}
	}
}
