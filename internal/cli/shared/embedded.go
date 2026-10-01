package shared

import (
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
)

var embedded fs.FS

// SetEmbeddedStd registers the std stubs compiled into the executable; dir
// is the directory inside fsys that holds prelude/, array/ and so on.
func SetEmbeddedStd(fsys fs.FS, dir string) {
	sub, err := fs.Sub(fsys, dir)
	if err == nil {
		embedded = sub
	}
}

// embeddedStdRoot unpacks the embedded std into the user cache once per
// content hash. The directory name is a public, offline-computable
// function of the binary's content, so an existing directory is re-hashed
// before it is trusted: anyone who can predict the name could pre-seed it
// with different content.
func embeddedStdRoot() string {
	if embedded == nil {
		return ""
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	hash := embeddedHash()
	dir := filepath.Join(cache, "kigumi", "std-"+hash)
	if validStdCache(dir, hash) {
		return dir
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), "std-")
	if err != nil {
		if os.MkdirAll(filepath.Dir(dir), 0o755) != nil {
			return ""
		}
		if tmp, err = os.MkdirTemp(filepath.Dir(dir), "std-"); err != nil {
			return ""
		}
	}
	if err := os.CopyFS(tmp, embedded); err != nil {
		os.RemoveAll(tmp)
		return ""
	}
	os.RemoveAll(dir)
	if err := os.Rename(tmp, dir); err != nil {
		os.RemoveAll(tmp)
		if validStdCache(dir, hash) {
			return dir
		}
		return ""
	}
	return dir
}

// validStdCache reports whether dir already holds an extraction that
// actually hashes to hash and is not writable by anyone but its owner.
func validStdCache(dir, hash string) bool {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() || st.Mode().Perm()&0o022 != 0 {
		return false
	}
	got, err := hashTree(dir)
	return err == nil && got == hash
}

// hashTree hashes an on-disk directory the same way embeddedHash hashes
// the embedded fs.FS, so the two are comparable.
func hashTree(dir string) (string, error) {
	h := fnv.New64a()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		h.Write([]byte(filepath.ToSlash(rel)))
		h.Write(b)
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum64()), nil
}

func embeddedHash() string {
	h := fnv.New64a()
	fs.WalkDir(embedded, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, _ := fs.ReadFile(embedded, path)
		h.Write([]byte(path))
		h.Write(b)
		return nil
	})
	return fmt.Sprintf("%x", h.Sum64())
}
