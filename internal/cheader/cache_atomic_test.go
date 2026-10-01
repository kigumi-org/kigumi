package cheader

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestWriteFileAtomicNeverExposesPartialFile runs concurrent readers and
// writers to catch a torn read from a non-atomic publish.
func TestWriteFileAtomicNeverExposesPartialFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gen.kg")
	oldLine := strings.Repeat("a", 4096) + "\n"
	newLine := strings.Repeat("b", 4096) + "\n"
	if err := writeFileAtomic(path, []byte(oldLine)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			if err := writeFileAtomic(path, []byte(newLine)); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			got := readSkipped(path)
			if got == nil {
				continue
			}
			line := got[0] + "\n"
			if line != oldLine && line != newLine {
				t.Errorf("torn read: got %d bytes starting %q", len(line), line[:min(16, len(line))])
				return
			}
		}
	}()
	wg.Wait()
}
