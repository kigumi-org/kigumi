package arch_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// envAllowed lists where a process environment may be read: the CLI
// boundary, which turns KIGUMI_* into explicit options, and the host
// intrinsics (interp and VM) that expose the environment to Kigumi programs.
var envAllowed = []string{"internal/cli/", "internal/interp/accel_host.go", "internal/interp/accel_os_time.go", "internal/vm/std_host.go"}

func TestNoEnvReads(t *testing.T) {
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
		rel := filepath.ToSlash(strings.TrimPrefix(p, "../../"))
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") || !strings.HasPrefix(rel, "internal/") {
			return nil
		}
		for _, ok := range envAllowed {
			if strings.HasPrefix(rel, ok) {
				return nil
			}
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, call := range []string{"os.Getenv(", "os.LookupEnv(", "os.Environ(", "os.ExpandEnv("} {
			if strings.Contains(string(b), call) {
				t.Errorf("%s: %s を読んでいる。環境変数は internal/cli で Options に変換せよ (plan §5.13)", rel, call)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
