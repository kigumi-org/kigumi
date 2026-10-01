package shared_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/spf13/cobra"

	"kigumi/internal/cli/shared"
)

func TestEmbeddedStdFallback(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KIGUMI_STD", "")
	shared.SetEmbeddedStd(fstest.MapFS{
		"std/prelude/prelude.kg": {Data: []byte("pub type Ordering = Less | Equal | Greater\n")},
		"std/array/array.kg":     {Data: []byte("pub type Array[T]\n")},
	}, "std")
	defer shared.SetEmbeddedStd(fstest.MapFS{}, ".")
	cmd := &cobra.Command{}
	cmd.Flags().String("std", "", "")
	root := shared.StdRoot(cmd)
	if root == "" {
		t.Fatal("no std root from the embedded copy")
	}
	if _, err := os.Stat(filepath.Join(root, "array", "array.kg")); err != nil {
		t.Fatalf("unpacked std incomplete: %v", err)
	}
	if again := shared.StdRoot(cmd); again != root {
		t.Fatalf("second lookup %q, want the cached %q", again, root)
	}
}

func TestEmbeddedStdRejectsPoisonedCache(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KIGUMI_STD", "")
	shared.SetEmbeddedStd(fstest.MapFS{
		"std/prelude/prelude.kg": {Data: []byte("pub type Ordering = Less | Equal | Greater\n")},
	}, "std")
	defer shared.SetEmbeddedStd(fstest.MapFS{}, ".")
	cmd := &cobra.Command{}
	cmd.Flags().String("std", "", "")
	root := shared.StdRoot(cmd)
	if root == "" {
		t.Fatal("no std root from the embedded copy")
	}

	os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "prelude"), 0o755); err != nil {
		t.Fatal(err)
	}
	poison := []byte("pub type Ordering = Poisoned\n")
	if err := os.WriteFile(filepath.Join(root, "prelude", "prelude.kg"), poison, 0o644); err != nil {
		t.Fatal(err)
	}

	again := shared.StdRoot(cmd)
	if again != root {
		t.Fatalf("lookup after poisoning %q, want the same path %q", again, root)
	}
	got, err := os.ReadFile(filepath.Join(again, "prelude", "prelude.kg"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pub type Ordering = Less | Equal | Greater\n" {
		t.Fatalf("used a poisoned cache instead of re-extracting: %q", got)
	}
}
