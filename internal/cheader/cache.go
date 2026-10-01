package cheader

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// formatVersion is folded into the cache key; bump it whenever Generate's
// output shape changes, so an old cache entry never masks a new one.
const formatVersion = 3

// cacheKey identifies one (header content, zig version, target, ABI)
// combination; a change in any of them must regenerate rather than reuse a
// stale translation.
func cacheKey(header []byte, zigVersion, target string, abi ABI) string {
	h := sha256.New()
	fmt.Fprintf(h, "kigumi-cheader-v%d\x00%s\x00%s\x00%v\x00", formatVersion, zigVersion, target, abi)
	h.Write(header)
	return hex.EncodeToString(h.Sum(nil))
}

func readSkipped(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func writeSkipped(path string, skipped []string) error {
	if len(skipped) == 0 {
		return nil
	}
	return writeFileAtomic(path, []byte(strings.Join(skipped, "\n")+"\n"))
}

// writeFileAtomic publishes through a temp file plus rename, so a concurrent
// reader on the shared, machine-wide cache directory never observes a partial
// write (TOCTOU; mirrors internal/driver/modio.go's same-named func).
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
