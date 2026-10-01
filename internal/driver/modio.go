package driver

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// OriginFileName marks a complete cache entry: `kigumi get` writes the
// commit it cloned into it last, so a directory without one is unfinished.
const OriginFileName = ".kigumi-origin"

// readFileMaybe reads path; ok is false only when the file does not exist.
// Any other failure (permissions, I/O) is an error, never "missing".
func readFileMaybe(path string) (data []byte, ok bool, err error) {
	data, err = os.ReadFile(path)
	switch {
	case err == nil:
		return data, true, nil
	case errors.Is(err, fs.ErrNotExist):
		return nil, false, nil
	}
	return nil, false, err
}

// writeFileAtomic publishes data at path through a temporary file in the
// same directory, so a crash or a concurrent reader never sees a partial
// file. An existing file's mode is preserved; perm applies only when path
// does not exist yet, so a chmod +x'd script does not lose its bit.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if fi, err := os.Stat(path); err == nil {
		perm = fi.Mode().Perm()
	}
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
	if err := os.Chmod(tmp.Name(), perm); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// WriteFileAtomic is writeFileAtomic for callers outside this package
// (`kigumi fmt -w`), which needs the same crash-safety and mode
// preservation for the user's own source files.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	return writeFileAtomic(path, data, perm)
}

// CheckCacheTree refuses a fetched module that holds anything but regular
// files and directories: a symlink could lead the compiler outside the
// cache, and devices or pipes are never module content.
func CheckCacheTree(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if t := d.Type(); t&fs.ModeSymlink != 0 || t&(fs.ModeDevice|fs.ModeNamedPipe|fs.ModeSocket|fs.ModeCharDevice) != 0 {
			rel, _ := filepath.Rel(dir, path)
			return fmt.Errorf("fetched module %s holds %s, which is not a regular file; refusing to use the cache entry", dir, filepath.ToSlash(rel))
		}
		return nil
	})
}

// CacheReady reports a complete cache entry: the origin marker and the
// manifest are both present.
func CacheReady(dir string) bool {
	_, okOrigin, _ := readFileMaybe(filepath.Join(dir, OriginFileName))
	_, okManifest, _ := readFileMaybe(filepath.Join(dir, ManifestName))
	return okOrigin && okManifest
}
