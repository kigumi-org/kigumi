package sem

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// A duplicate code number would make `--explain` and the docs ambiguous,
// so every defineCode number must be unique.
func TestCodeNumbersUnique(t *testing.T) {
	re := regexp.MustCompile(`defineCode\("(E\d+)", "([a-z0-9-]+)"`)
	seen := map[string]string{}
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			if prev, ok := seen[m[1]]; ok && prev != m[2] {
				t.Errorf("%s defined twice: %s and %s", m[1], prev, m[2])
			}
			seen[m[1]] = m[2]
		}
	}
}
