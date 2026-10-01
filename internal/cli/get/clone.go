package get

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"kigumi/internal/driver"
	"kigumi/internal/errors"
)

// clone fetches the version of r into dir through a temporary directory,
// so a failed fetch leaves no half-written module, and returns the commit
// it resolved to. Tags are `<subdir>/<version>` for nested modules; a hex
// version is a commit, expanded from its short form in the clone.
func clone(cmd *cobra.Command, r driver.Require, dir string) (string, error) {
	url, subdir, ok := driver.RepoOf(r)
	if !ok {
		return "", errors.NewErrf("dependency %s %s: cannot derive its repository; give `url` in the Require", r.Name, r.Version)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", errors.WrapErr(err, "create module cache")
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), "fetch-")
	if err != nil {
		return "", errors.WrapErr(err, "create module cache")
	}
	defer os.RemoveAll(tmp)
	var commit string
	if driver.IsCommitHash(r.Version) {
		commit, err = cloneCommit(cmd, url, r.Version, tmp)
	} else {
		commit, err = cloneTag(cmd, url, driver.TagOf(subdir, r.Version), tmp)
	}
	if err != nil {
		return "", err
	}
	src := filepath.Join(tmp, filepath.FromSlash(subdir))
	if _, err := os.Stat(filepath.Join(src, driver.ManifestName)); err != nil {
		return "", errors.NewErrf("%s at %s has no %s in %q", r.Name, r.Version, driver.ManifestName, subdir)
	}
	os.RemoveAll(filepath.Join(tmp, ".git"))
	if err := dropNestedModules(src); err != nil {
		return "", err
	}
	if err := driver.CheckCacheTree(src); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(src, originFile), []byte(commit+"\n"), 0o644); err != nil {
		return "", errors.WrapErr(err, "record origin")
	}
	if err := os.Rename(src, dir); err != nil {
		// Another process may have published the same entry meanwhile.
		if driver.CacheReady(dir) {
			return commit, nil
		}
		return "", errors.WrapErr(err, "store fetched module")
	}
	return commit, nil
}

func git(cmd *cobra.Command, dir string, args ...string) (string, error) {
	g := exec.Command("git", args...)
	g.Dir = dir
	g.Stderr = cmd.ErrOrStderr()
	out, err := g.Output()
	return strings.TrimSpace(string(out)), err
}

func cloneTag(cmd *cobra.Command, url, tag, tmp string) (string, error) {
	if _, err := git(cmd, "", "clone", "--quiet", "--depth", "1", "--branch", tag, url, tmp); err != nil {
		return "", errors.WrapErrf(err, "git clone %s at %s", url, tag)
	}
	head, err := git(cmd, tmp, "rev-parse", "HEAD")
	if err != nil {
		return "", errors.WrapErr(err, "git rev-parse")
	}
	// --branch also matches a branch of that name; insist on the tag.
	if tagged, err := git(cmd, tmp, "rev-parse", "--verify", "--quiet", "refs/tags/"+tag+"^{commit}"); err != nil || tagged != head {
		return "", errors.NewErrf("%s has no tag %s (a branch of that name is not a version)", url, tag)
	}
	return head, nil
}

func cloneCommit(cmd *cobra.Command, url, hash, tmp string) (string, error) {
	if _, err := git(cmd, "", "clone", "--quiet", "--no-checkout", url, tmp); err != nil {
		return "", errors.WrapErrf(err, "git clone %s", url)
	}
	full, err := git(cmd, tmp, "rev-parse", "--verify", "--quiet", hash+"^{commit}")
	if err != nil {
		return "", errors.NewErrf("%s has no commit %s (or it is ambiguous)", url, hash)
	}
	if _, err := git(cmd, tmp, "checkout", "--quiet", full); err != nil {
		return "", errors.WrapErrf(err, "git checkout %s", full)
	}
	return full, nil
}

// dropNestedModules removes module roots below src: they are separate
// modules with their own versions and are fetched on their own.
func dropNestedModules(src string) error {
	var nested []string
	filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == src {
			return nil
		}
		if driver.IsModuleRoot(path) {
			nested = append(nested, path)
			return fs.SkipDir
		}
		return nil
	})
	for _, n := range nested {
		if err := os.RemoveAll(n); err != nil {
			return errors.WrapErr(err, "drop nested module")
		}
	}
	return nil
}

// latestTag asks the repo for its tags and picks the highest release
// version of the module's subdirectory (prereleases excluded).
func latestTag(r driver.Require) (string, error) {
	url, subdir, _ := driver.RepoOf(r)
	out, err := exec.Command("git", "ls-remote", "--tags", "--refs", url).Output()
	if err != nil {
		return "", errors.WrapErrf(err, "git ls-remote %s", url)
	}
	prefix := "refs/tags/" + driver.TagOf(subdir, "")
	var versions []string
	for _, line := range bytes.Split(out, []byte("\n")) {
		_, ref, ok := strings.Cut(string(line), "\t")
		if v, has := strings.CutPrefix(ref, prefix); ok && has && strings.HasPrefix(v, "v") && !strings.Contains(v, "-") && !strings.Contains(v, "/") {
			versions = append(versions, v)
		}
	}
	if len(versions) == 0 {
		return "", errors.NewErrf("%s has no release tag for %s", url, r.Name)
	}
	sort.Slice(versions, func(i, j int) bool { return driver.CompareVersions(versions[i], versions[j]) < 0 })
	return versions[len(versions)-1], nil
}
