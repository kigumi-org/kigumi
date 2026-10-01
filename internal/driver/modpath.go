package driver

import (
	"fmt"
	"os"

	"kigumi/internal/version"

	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ToolchainVersion is what `Module { kigumi }` is compared against; a module
// written for a newer toolchain is refused instead of misparsed.
const ToolchainVersion = version.Toolchain

// knownHosts are forges where the first three path segments name the repo;
// elsewhere a `.git` segment marks the boundary.
var knownHosts = map[string]bool{"github.com": true, "gitlab.com": true, "bitbucket.org": true, "codeberg.org": true, "sr.ht": true}

// HasHost reports whether a module name starts with a host (a dotted first
// segment), so its repo can be derived.
func HasHost(name string) bool {
	first, _, _ := strings.Cut(name, "/")
	return strings.Contains(first, ".")
}

// RepoOf derives the clone URL and the subdirectory of a module name.
// A Require's explicit url wins; the subdirectory is then what the name
// carries beyond the url's host and path.
func RepoOf(r Require) (url, subdir string, ok bool) {
	if r.URL != "" {
		return r.URL, subdirUnder(r.Name, r.URL), true
	}
	segs := strings.Split(r.Name, "/")
	if !HasHost(r.Name) {
		return "", "", false
	}
	if knownHosts[strings.ToLower(segs[0])] && len(segs) >= 3 {
		return "https://" + strings.Join(segs[:3], "/") + ".git", strings.Join(segs[3:], "/"), true
	}
	for i, s := range segs {
		if strings.HasSuffix(s, ".git") && i > 0 {
			return "https://" + strings.Join(segs[:i+1], "/"), strings.Join(segs[i+1:], "/"), true
		}
	}
	return "", "", false
}

// subdirUnder strips the repo part of a url from a module name: the
// longest tail of the url's path that starts the name, segment-wise, so
// both `https://h/a/b.git` and a local `/tmp/h/a/b` checkout of
// `h/a/b/sub` give `sub`. No overlap means the repo root.
func subdirUnder(name, url string) string {
	u := url
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	if i := strings.Index(u, "@"); i >= 0 && !strings.Contains(u[:i], "/") {
		u = u[i+1:]
	}
	u = strings.TrimSuffix(strings.Replace(u, ":", "/", 1), "/")
	for _, form := range []string{u, strings.TrimSuffix(u, ".git")} {
		segs := strings.Split(form, "/")
		for i := range segs {
			tail := strings.Join(segs[i:], "/")
			if tail == "" {
				continue
			}
			if name == tail {
				return ""
			}
			if rest, ok := strings.CutPrefix(name, tail+"/"); ok {
				return rest
			}
		}
	}
	return ""
}

// TagOf is the git tag of a version: nested modules tag as
// `<subdir>/<version>`.
func TagOf(subdir, version string) string {
	if subdir == "" {
		return version
	}
	return subdir + "/" + version
}

var commitHash = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// IsCommitHash tells a commit version from a tag: hex only, 7 to 40 digits.
func IsCommitHash(version string) bool { return commitHash.MatchString(version) }

// CacheDir is where a fetched module lives: <cache>/kigumi/mod/<name>@<version>
// with upper-case letters escaped as `!x`, so names differing only in case
// stay apart on case-insensitive file systems (as Go does).
func CacheDir(r Require) (string, error) {
	if err := ValidModuleName(r.Name); err != nil {
		return "", err
	}
	if err := ValidVersion(r.Version); err != nil {
		return "", err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "kigumi", "mod", filepath.FromSlash(escapePath(r.Name))+"@"+escapePath(r.Version)), nil
}

func escapePath(s string) string {
	var sb strings.Builder
	for _, c := range s {
		if c >= 'A' && c <= 'Z' {
			sb.WriteByte('!')
			sb.WriteRune(c + ('a' - 'A'))
			continue
		}
		sb.WriteRune(c)
	}
	return sb.String()
}

// ownerOf picks the module a path belongs to: the longest name that is the
// path itself or a segment-wise prefix of it.
func ownerOf(path string, names []string) string {
	best := ""
	for _, n := range names {
		if (path == n || strings.HasPrefix(path, n+"/")) && len(n) > len(best) {
			best = n
		}
	}
	return best
}

// versionAtLeast compares dotted numeric versions ("0.4" >= "0.3.1"). want
// is a manifest's `kigumi` field, so a non-numeric component is reported
// instead of silently comparing as 0.
func versionAtLeast(have, want string) (bool, error) {
	h, w := strings.Split(have, "."), strings.Split(want, ".")
	for i := 0; i < len(h) || i < len(w); i++ {
		var a, b int
		var err error
		if i < len(h) {
			if a, err = strconv.Atoi(h[i]); err != nil {
				return false, fmt.Errorf("version %q: component %q is not a number", have, h[i])
			}
		}
		if i < len(w) {
			if b, err = strconv.Atoi(w[i]); err != nil {
				return false, fmt.Errorf("version %q: component %q is not a number", want, w[i])
			}
		}
		if a != b {
			return a > b, nil
		}
	}
	return true, nil
}
