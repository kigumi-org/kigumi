package arch_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const maxLines = 200

func TestFileSize(t *testing.T) {
	err := filepath.WalkDir("../..", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" || d.Name() == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if n := bytes.Count(b, []byte("\n")); n > maxLines {
			t.Errorf("%s: %d行(上限%d)。動詞_名詞.goで分割せよ", p, n, maxLines)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
