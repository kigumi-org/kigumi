// Package get implements `kigumi get`: fetch the dependencies of a module
// or script (and theirs) into the cache and record them in the lock file,
// or add one Require and fetch it.
package get

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"kigumi/internal/cli/shared"
	"kigumi/internal/cliutil"
	"kigumi/internal/driver"
	"kigumi/internal/errors"
)

func Cmd() *cobra.Command {
	var update, lock bool
	var url string
	cmd := &cobra.Command{
		Use:   "get [<module path>[@<version>]] [<module-root>|<script.kg>]",
		Short: "Fetch dependencies with git and write the lock file; with a module path, add it first",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, add := ".", ""
			for _, a := range args {
				if st, err := os.Stat(a); err == nil && (st.IsDir() || strings.HasSuffix(a, ".kg")) {
					target = a
				} else if strings.HasSuffix(a, ".kg") {
					return &cliutil.UsageError{Err: errors.NewErrf("no such file: %s", a)}
				} else if add == "" {
					add = a
				} else {
					return &cliutil.UsageError{Err: errors.NewErrf("%s is neither a module path nor an existing module root or script", a)}
				}
			}
			t, err := openTarget(target, lock)
			if err != nil {
				return err
			}
			if add != "" {
				if err := t.add(cmd, add, url); err != nil {
					return err
				}
			}
			return t.fetch(cmd, update)
		},
	}
	cmd.Flags().BoolVarP(&update, "update", "u", false, "accept a tag that moved since the lock file was written")
	cmd.Flags().BoolVar(&lock, "lock", false, "for a script, write <script>.lock.kg next to it")
	cmd.Flags().StringVar(&url, "url", "", "clone URL of the module being added, when it cannot be derived from its name")
	return cmd
}

// target is what `kigumi get` operates on: a module root with mod.kg, or a
// script carrying manifest records.
type target struct {
	dir, script, lockPath string
	manifest              driver.ModFile
}

func openTarget(path string, lock bool) (*target, error) {
	if strings.HasSuffix(path, ".kg") {
		mf, _, err := driver.ReadScriptManifest(path)
		if err != nil {
			return nil, errors.WrapErr(err, "read script manifest")
		}
		t := &target{dir: filepath.Dir(path), script: path, manifest: mf}
		if lock || fileExists(path+".lock.kg") {
			t.lockPath = path + ".lock.kg"
		}
		return t, nil
	}
	mf, ok, err := driver.ReadModFile(path)
	if err != nil {
		return nil, errors.WrapErr(err, "read manifest")
	}
	if !ok {
		return nil, &cliutil.UsageError{Err: errors.NewErrf("%s has no %s; run `kigumi mod init` or name a script", path, driver.ManifestName)}
	}
	return &target{dir: path, manifest: mf, lockPath: filepath.Join(path, driver.LockName)}, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// add appends a Require for `path[@version]`, asking the repo for its
// newest tag when no version is given.
func (t *target) add(cmd *cobra.Command, spec, url string) error {
	name, version, _ := strings.Cut(spec, "@")
	r := driver.Require{Name: name, Version: version, URL: url}
	if _, _, ok := driver.RepoOf(r); !ok {
		return &cliutil.UsageError{Err: errors.NewErrf("cannot derive where %s lives; give --url", name)}
	}
	if r.Version == "" {
		v, err := latestTag(r)
		if err != nil {
			return err
		}
		r.Version = v
	}
	var err error
	if t.script != "" {
		err = driver.AppendScriptRequire(t.script, r)
	} else {
		err = driver.AppendRequire(t.dir, r)
	}
	if err != nil {
		return &cliutil.UsageError{Err: err}
	}
	cmd.Printf("added %s %s\n", r.Name, r.Version)
	t.manifest.Requires = append(t.manifest.Requires, r)
	return nil
}

func (t *target) fetch(cmd *cobra.Command, update bool) error {
	lock := driver.Lock{Entries: map[string]driver.LockEntry{}}
	if t.lockPath != "" {
		var err error
		if lock, err = driver.ReadLockFile(t.lockPath); err != nil {
			return errors.WrapErr(err, "read lock")
		}
	}
	local, err := driver.ReadLocal(t.dir)
	if err != nil {
		return errors.WrapErr(err, "read local overrides")
	}
	if shared.LocalOff() {
		local = driver.Local{Replaces: map[string]string{}}
	}
	if noLocal, _ := cmd.Flags().GetBool("no-local"); noLocal {
		local = driver.Local{Replaces: map[string]string{}}
	}
	f := &fetcher{cmd: cmd, update: update, lock: lock, local: local, next: driver.Lock{Entries: map[string]driver.LockEntry{}}}
	if err := f.module(t.manifest); err != nil {
		return err
	}
	if t.lockPath == "" {
		return nil
	}
	if err := driver.WriteLockFile(t.lockPath, f.next); err != nil {
		return errors.WrapErr(err, "write lock")
	}
	return nil
}
