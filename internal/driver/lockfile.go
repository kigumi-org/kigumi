package driver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kigumi/internal/diag"
	"kigumi/internal/token"
)

// Lock is mod.lock.kg: what each fetched Require resolved to, keyed by
// module name. A Replace'd dependency has no entry.
//
//	Lock {
//	    name: "greet"
//	    version: "v0.1.0"
//	    commit: "3f2c..."
//	    hash: "h1:9a0b..."
//	}
type Lock struct {
	Entries map[string]LockEntry
}

type LockEntry struct {
	Version, Commit, Hash string
}

var lockFields = map[string][]string{"Lock": {"name", "version", "commit", "hash"}}

// ReadLock parses root/mod.lock.kg; a missing file is an empty lock.
func ReadLock(root string) (Lock, error) { return ReadLockFile(filepath.Join(root, LockName)) }

// ReadLockFile parses a lock file at path (a script's `<file>.lock.kg`
// sits next to the script).
func ReadLockFile(path string) (Lock, error) {
	src, ok, err := readFileMaybe(path)
	if err != nil {
		return Lock{Entries: map[string]LockEntry{}}, err
	}
	if !ok {
		return Lock{Entries: map[string]LockEntry{}}, nil
	}
	f := token.NewFile(filepath.Base(path), src)
	lk, ds := ParseLock(f)
	if len(ds) > 0 {
		return lk, fmt.Errorf("%s: %s", path, diag.RenderAll(f, ds[:1]))
	}
	return lk, nil
}

// ParseLock decodes a lock file into positioned diagnostics on failure.
func ParseLock(f *token.File) (Lock, []diag.Diagnostic) {
	lk := Lock{Entries: map[string]LockEntry{}}
	recs, ds := parseRecords(f, lockFields)
	for _, r := range recs {
		if _, dup := lk.Entries[r.Fields["name"]]; dup {
			ds = append(ds, diag.Errorf(diag.Location{Span: r.Span}, fmt.Sprintf("%s is locked twice", r.Fields["name"])))
		}
		lk.Entries[r.Fields["name"]] = LockEntry{Version: r.Fields["version"], Commit: r.Fields["commit"], Hash: r.Fields["hash"]}
	}
	return lk, ds
}

// WriteLock rewrites root/mod.lock.kg sorted by name; an empty lock removes
// the file.
func WriteLock(root string, lk Lock) error { return WriteLockFile(filepath.Join(root, LockName), lk) }

// WriteLockFile writes a lock to an explicit path (a script's lock).
func WriteLockFile(path string, lk Lock) error {
	if len(lk.Entries) == 0 {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	names := make([]string, 0, len(lk.Entries))
	for n := range lk.Entries {
		names = append(names, n)
	}
	sort.Strings(names)
	var sb strings.Builder
	sb.WriteString("// Written by `kigumi get`: the commit and tree hash each Require resolved to.\n")
	for _, n := range names {
		e := lk.Entries[n]
		sb.WriteString("\n")
		writeRecord(&sb, "Lock", "name", n, "version", e.Version, "commit", e.Commit, "hash", e.Hash)
	}
	return writeFileAtomic(path, []byte(sb.String()), 0o644)
}

// TreeHash digests every regular file under dir (path and content) into
// "h1:<sha256 hex>", so a tampered or truncated cache entry is detectable.
func TreeHash(dir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(h, "%s\x00%x\n", filepath.ToSlash(rel), sum)
		return nil
	})
	if err != nil {
		return "", err
	}
	return "h1:" + hex.EncodeToString(h.Sum(nil)), nil
}
