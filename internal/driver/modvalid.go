package driver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Module names and versions become cache paths, git refs and URLs, so
// they are validated before any of those is built from them.

var nameSegment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidModuleName checks the host-style path form: `/`-separated ASCII
// segments of letters, digits, `.`, `-` and `_`, none empty, `.` or `..`.
func ValidModuleName(name string) error {
	if name == "" {
		return fmt.Errorf("module name is empty")
	}
	if len(name) > 255 {
		return fmt.Errorf("module name %q is longer than 255 bytes", name)
	}
	for _, seg := range strings.Split(name, "/") {
		switch {
		case seg == "":
			return fmt.Errorf("module name %q has an empty path segment", name)
		case seg == "." || seg == "..":
			return fmt.Errorf("module name %q has a `%s` segment", name, seg)
		case !nameSegment.MatchString(seg):
			return fmt.Errorf("module name %q: segment %q may only use letters, digits, `.`, `-` and `_`", name, seg)
		}
	}
	return nil
}

var semverTag = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

// ValidVersion accepts a `v`-prefixed SemVer 2.0 tag or a commit hash.
func ValidVersion(v string) error {
	if IsCommitHash(v) {
		return nil
	}
	if _, err := parseSemVer(v); err != nil {
		return err
	}
	return nil
}

type semVer struct {
	nums [3]int64
	pre  []string
}

func parseSemVer(v string) (semVer, error) {
	m := semverTag.FindStringSubmatch(v)
	if m == nil {
		return semVer{}, fmt.Errorf("version %q is not `v<major>.<minor>.<patch>[-prerelease][+build]` or a commit hash", v)
	}
	var out semVer
	for i := range 3 {
		n, err := strconv.ParseInt(m[i+1], 10, 64)
		if err != nil {
			return semVer{}, fmt.Errorf("version %q: component %q does not fit", v, m[i+1])
		}
		out.nums[i] = n
	}
	if m[4] != "" {
		out.pre = strings.Split(m[4], ".")
		for _, id := range out.pre {
			if numeric(id) && len(id) > 1 && id[0] == '0' {
				return semVer{}, fmt.Errorf("version %q: prerelease identifier %q has a leading zero", v, id)
			}
		}
	}
	return out, nil
}

func numeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

// CompareVersions orders two SemVer tags as SemVer 2.0 does: numeric
// components, then prerelease identifiers (numeric before alphanumeric,
// numeric ones by value), a release above its prereleases; build metadata
// is ignored. A malformed tag sorts by plain string comparison so that a
// selection never panics on manifests that validation already reported.
func CompareVersions(a, b string) int {
	va, ea := parseSemVer(a)
	vb, eb := parseSemVer(b)
	if ea != nil || eb != nil {
		return strings.Compare(a, b)
	}
	for i := range 3 {
		if va.nums[i] != vb.nums[i] {
			if va.nums[i] < vb.nums[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(va.pre) == 0 && len(vb.pre) == 0:
		return 0
	case len(va.pre) == 0:
		return 1
	case len(vb.pre) == 0:
		return -1
	}
	for i := 0; i < len(va.pre) && i < len(vb.pre); i++ {
		if c := comparePre(va.pre[i], vb.pre[i]); c != 0 {
			return c
		}
	}
	return compareInts(len(va.pre), len(vb.pre))
}

// comparePre orders two prerelease identifiers per SemVer 2.0: numeric
// ones by value, numeric below alphanumeric, alphanumeric lexically.
// parseSemVer already rejects a leading zero on a numeric identifier, so
// two of them compare by digit count then digit string, with no int64
// conversion to overflow.
func comparePre(x, y string) int {
	nx, ny := numeric(x), numeric(y)
	switch {
	case nx && ny:
		if c := compareInts(len(x), len(y)); c != 0 {
			return c
		}
		return strings.Compare(x, y)
	case nx:
		return -1
	case ny:
		return 1
	}
	return strings.Compare(x, y)
}

func compareInts(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// majorOf is the major component of a tag; -1 for a commit hash.
func majorOf(v string) int64 {
	sv, err := parseSemVer(v)
	if err != nil {
		return -1
	}
	return sv.nums[0]
}
