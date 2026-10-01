package driver

import (
	"fmt"
	"sort"
	"strings"
)

// Selection is the result of minimal version selection: for every
// module reached through Require records, the highest version any path
// asked for. Versions are semver tags; a commit hash only agrees with an
// identical hash.
type Selection struct {
	Requires map[string]Require
	// By names the module that asked for the selected version.
	By map[string]string
}

// MissingError reports a module whose manifest is needed to continue the
// selection but is not in the cache yet; `kigumi get` fetches it and retries.
type MissingError struct{ Require Require }

func (e *MissingError) Error() string {
	return fmt.Sprintf("dependency %s %s is not fetched; run `kigumi get`", e.Require.Name, e.Require.Version)
}

// verKey identifies one manifest: a module name at one exact version.
type verKey struct{ name, version string }

// SelectVersions runs minimal version selection in two passes: pass 1 reads
// every reachable (name, version) manifest so the maximum per name does not
// depend on discovery order; pass 2 rebuilds the edges from the winning
// manifests only, so dependencies reachable only through a loser drop out.
func SelectVersions(root ModFile, local Local) (*Selection, error) {
	max := map[string]Require{}
	by := map[string]string{}
	manifests := map[verKey]ModFile{}
	visited := map[verKey]bool{}
	var queue []Require

	visit := func(r Require, from string) error {
		if cur, ok := max[r.Name]; !ok {
			max[r.Name], by[r.Name] = r, from
		} else if r.Version == cur.Version {
			if r.URL != "" && cur.URL != "" && r.URL != cur.URL {
				return fmt.Errorf("dependency %s %s is required from %s by %s and from %s by %s", r.Name, r.Version, cur.URL, by[r.Name], r.URL, from)
			}
		} else if CompareVersions(r.Version, cur.Version) == 0 {
			// Same precedence, different literal tag (build metadata):
			// CacheDir keys off the literal string, so these are not
			// necessarily the same content and must not merge silently.
			return fmt.Errorf("dependency %s is required at %s by %s and at %s by %s: same precedence but different tags", r.Name, cur.Version, by[r.Name], r.Version, from)
		} else {
			newer, err := versionNewer(r, cur)
			if err != nil {
				return fmt.Errorf("dependency %s is required at %s by %s and at %s by %s: %w", r.Name, cur.Version, by[r.Name], r.Version, from, err)
			}
			if newer {
				if r.URL == "" {
					r.URL = cur.URL
				}
				max[r.Name], by[r.Name] = r, from
			}
		}
		key := verKey{r.Name, r.Version}
		if visited[key] {
			return nil
		}
		visited[key] = true
		queue = append(queue, r)
		return nil
	}

	for _, r := range root.Requires {
		if err := visit(r, byName(root)); err != nil {
			return nil, err
		}
	}
	for len(queue) > 0 {
		r := queue[0]
		queue = queue[1:]
		dir, missing, err := DepDir(local, r, true)
		if err != nil {
			return nil, err
		}
		if missing {
			return nil, &MissingError{Require: r}
		}
		mf, ok, err := ReadModFile(dir)
		if err != nil {
			return nil, err
		}
		if ok && mf.Name != "" && mf.Name != r.Name {
			return nil, fmt.Errorf("dependency %s: its %s names the module %q", r.Name, ManifestName, mf.Name)
		}
		manifests[verKey{r.Name, r.Version}] = mf
		for _, sub := range mf.Requires {
			if err := visit(sub, r.Name); err != nil {
				return nil, err
			}
		}
	}

	root2 := byName(root)
	edges := map[string][]string{}
	for _, r := range root.Requires {
		edges[root2] = append(edges[root2], r.Name)
	}
	for name, r := range max {
		mf := manifests[verKey{name, r.Version}]
		for _, sub := range mf.Requires {
			edges[name] = append(edges[name], sub.Name)
		}
	}
	if chain := findCycle(edges, root2); chain != nil {
		return nil, fmt.Errorf("dependency cycle: %s", strings.Join(chain, " -> "))
	}

	reachable := map[string]bool{}
	var walk func(n string)
	walk = func(n string) {
		if reachable[n] {
			return
		}
		reachable[n] = true
		for _, m := range edges[n] {
			walk(m)
		}
	}
	walk(root2)

	sel := &Selection{Requires: map[string]Require{}, By: map[string]string{}}
	for name, r := range max {
		if reachable[name] {
			sel.Requires[name], sel.By[name] = r, by[name]
		}
	}
	return sel, nil
}

// findCycle walks the manifest graph from root and returns the first cycle
// as a chain ending where it began.
func findCycle(edges map[string][]string, root string) []string {
	const visiting, done = 1, 2
	state := map[string]int{}
	var path []string
	var visit func(n string) []string
	visit = func(n string) []string {
		state[n] = visiting
		path = append(path, n)
		for _, m := range edges[n] {
			switch state[m] {
			case visiting:
				for i, p := range path {
					if p == m {
						return append(append([]string{}, path[i:]...), m)
					}
				}
			case 0:
				if c := visit(m); c != nil {
					return c
				}
			}
		}
		path = path[:len(path)-1]
		state[n] = done
		return nil
	}
	return visit(root)
}

// Names lists the selected modules in a stable order.
func (s *Selection) Names() []string {
	names := make([]string, 0, len(s.Requires))
	for n := range s.Requires {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// versionNewer says whether a is a newer tag than b. Two commit hashes
// only agree when equal, and a hash never compares with a tag.
func versionNewer(a, b Require) (bool, error) {
	if a.Version == b.Version {
		return false, nil
	}
	if IsCommitHash(a.Version) || IsCommitHash(b.Version) {
		return false, fmt.Errorf("a commit hash cannot be reconciled with another version")
	}
	// Without a major suffix in module names, a higher major is
	// a different API, not a newer version of the same one.
	if majorOf(a.Version) != majorOf(b.Version) {
		return false, fmt.Errorf("different major versions cannot be mixed")
	}
	return CompareVersions(a.Version, b.Version) > 0, nil
}
