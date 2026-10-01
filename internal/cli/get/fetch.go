package get

import (
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"kigumi/internal/driver"
	"kigumi/internal/errors"
)

// originFile keeps the commit a cache entry was cloned from; the .git
// directory itself is removed to keep the cache small.
const originFile = driver.OriginFileName

type fetcher struct {
	cmd    *cobra.Command
	update bool
	// lock is what the lock file said before; next is what it will say.
	lock, next driver.Lock
	local      driver.Local
}

// module fetches until minimal version selection over the manifests
// closes. A Replace from mod.local.kg applies to the root and is never
// locked.
func (f *fetcher) module(mf driver.ModFile) error {
	var sel *driver.Selection
	for {
		var err error
		sel, err = driver.SelectVersions(mf, f.local)
		if err == nil {
			break
		}
		var missing *driver.MissingError
		if !stderrors.As(err, &missing) {
			return err
		}
		dir, _, err := driver.DepDir(f.local, missing.Require, true)
		if err != nil {
			return err
		}
		if err := f.cached(missing.Require, dir, true); err != nil {
			return err
		}
	}
	for _, name := range sel.Names() {
		r := sel.Requires[name]
		dir, missing, err := driver.DepDir(f.local, r, true)
		if err != nil {
			return errors.WrapErr(err, "resolve dependency")
		}
		if _, replaced := f.local.Replaces[r.Name]; replaced {
			f.cmd.Printf("%s %s: %s (replace)\n", r.Name, r.Version, dir)
		} else if err := f.cached(r, dir, missing); err != nil {
			return err
		}
		if depMf, ok, err := driver.ReadModFile(dir); err != nil {
			return errors.WrapErr(err, "read dependency manifest")
		} else if ok {
			f.native(r.Name, depMf)
		}
	}
	return nil
}

// native lists what a dependency adds to the link line, so the root sees
// what it is trusting.
func (f *fetcher) native(name string, mf driver.ModFile) {
	for _, l := range mf.Links {
		if l.Library != "" {
			f.cmd.Printf("  %s links -l%s\n", name, l.Library)
		}
		if l.Search != "" {
			f.cmd.Printf("  %s adds search path %s\n", name, l.Search)
		}
	}
	for _, s := range mf.Sources {
		f.cmd.Printf("  %s compiles %s\n", name, s.Path)
	}
}

// cached makes sure the cache holds r and that it agrees with the lock.
func (f *fetcher) cached(r driver.Require, dir string, missing bool) error {
	if _, done := f.next.Entries[r.Name]; done {
		return nil
	}
	prev, locked := f.lock.Entries[r.Name]
	locked = locked && prev.Version == r.Version
	if missing {
		commit, err := clone(f.cmd, r, dir)
		if err != nil {
			return err
		}
		if locked && !f.update && prev.Commit != commit {
			os.RemoveAll(dir)
			return errors.NewErrf("%s %s now points at %s but the lock recorded %s; run `kigumi get -u` to accept the new commit", r.Name, r.Version, short(commit), short(prev.Commit))
		}
		f.cmd.Printf("%s %s: fetched into %s\n", r.Name, r.Version, dir)
	} else {
		f.cmd.Printf("%s %s: %s\n", r.Name, r.Version, dir)
	}
	hash, err := driver.TreeHash(dir)
	if err != nil {
		return errors.WrapErr(err, "hash dependency")
	}
	if locked && prev.Hash != hash && !(missing && f.update) {
		return errors.NewErrf("%s %s in %s does not match the lock; remove that directory and run `kigumi get` again", r.Name, r.Version, dir)
	}
	commit, err := os.ReadFile(filepath.Join(dir, originFile))
	if err != nil {
		return errors.WrapErrf(err, "%s %s: read the origin marker", r.Name, r.Version)
	}
	f.next.Entries[r.Name] = driver.LockEntry{Version: r.Version, Commit: strings.TrimSpace(string(commit)), Hash: hash}
	return nil
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
