package driver

import (
	"fmt"
	"os"
	"path/filepath"
)

// DepDir resolves where a required module is on disk: the developer's
// local override (root module only) or the fetched copy in the cache.
// missing is true when the cache has no copy yet.
func DepDir(local Local, r Require, isRoot bool) (dir string, missing bool, err error) {
	if p, ok := local.Replaces[r.Name]; ok && isRoot {
		return p, false, nil
	}
	dir, err = CacheDir(r)
	if err != nil {
		return "", false, err
	}
	return dir, !CacheReady(dir), nil
}

// loadDeps loads the modules minimal version selection picks. enforceLock
// is false only for a script without <script>.lock.kg: its lock is opt-in.
func (m *Module) loadDeps(opts LoadOptions, lock Lock, enforceLock bool) error {
	for _, r := range m.Manifest.Requires {
		if r.Name == m.Name {
			return fmt.Errorf("dependency %s overlaps this module's name %s", r.Name, m.Name)
		}
	}
	sel, err := SelectVersions(m.Manifest, m.local)
	if err != nil {
		return err
	}
	m.Selected = sel.Requires
	// The root's own packages carry Module=="" too, same as a dependency
	// package not yet attributed below; snapshotting them here keeps
	// a dependency name that prefixes the root's own from claiming them.
	rootOwned := make(map[string]bool, len(m.Packages))
	for p := range m.Packages {
		rootOwned[p] = true
	}
	for _, name := range sel.Names() {
		r := sel.Requires[name]
		depDir, _, err := DepDir(m.local, r, true)
		if err != nil {
			return err
		}
		if _, replaced := m.local.Replaces[r.Name]; !replaced {
			if err := CheckCacheTree(depDir); err != nil {
				return err
			}
			entry, locked := lock.Entries[r.Name]
			if !locked || entry.Version != r.Version {
				if enforceLock {
					return fmt.Errorf("%s %s is not in %s; run `kigumi get` to lock it", r.Name, r.Version, LockName)
				}
			} else {
				hash, err := TreeHash(depDir)
				if err != nil {
					return err
				}
				if hash != entry.Hash {
					return fmt.Errorf("%s %s in %s does not match %s; the cache may be corrupted or tampered with, remove that directory and run `kigumi get` again", r.Name, entry.Version, depDir, LockName)
				}
			}
		}
		depMod, _, err := ReadModFile(depDir)
		if err != nil {
			return err
		}
		if err := m.addNative(depDir, depMod); err != nil {
			return err
		}
		if err := m.addHeaders(depDir, depMod); err != nil {
			return err
		}
		// A dependency's cmd/ holds its own scripts and tools: entries of that module, not packages of ours.
		depCmd := filepath.Join(depDir, "cmd")
		skip := func(d string) bool { return skipDir(d, opts.StdRoot) || IsModuleRoot(d) || sameDir(d, depCmd) }
		// A name-prefix relation alone is not an overlap; only an
		// actual package-path collision is.
		if overlap := m.dependencyOverlap(depDir, r.Name, skip); overlap != "" {
			return fmt.Errorf("dependency %s overlaps this module's name %s", r.Name, m.Name)
		}
		if err := m.loadTree(depDir, r.Name, false, false, skip); err != nil {
			return err
		}
		for _, p := range m.Packages {
			if p.Module == "" && !p.Std && !rootOwned[p.Path] && ownerOf(p.Path, []string{r.Name}) != "" {
				p.Module = r.Name
			}
		}
	}
	return nil
}

// addNative collects a module's C libraries and sources, resolved against
// its directory. The manifest's lexical check cannot see symlinks, so each
// resolved path is confined to the directory again here.
func (m *Module) addNative(dir string, mf ModFile) error {
	for _, l := range mf.Links {
		if l.Search != "" {
			l.Search = filepath.Join(dir, filepath.FromSlash(l.Search))
			if !withinSandbox([]string{dir}, l.Search) {
				return fmt.Errorf("%s: Link search path %q resolves outside the module", mf.Name, l.Search)
			}
		}
		m.Links = append(m.Links, l)
	}
	for _, s := range mf.Sources {
		for _, f := range m.selectSources(filepath.Join(dir, filepath.FromSlash(s.Path)), s.Asm) {
			if !withinSandbox([]string{dir}, f) {
				rel, _ := filepath.Rel(dir, f)
				return fmt.Errorf("%s: Source %q resolves outside the module", mf.Name, filepath.ToSlash(rel))
			}
			m.Sources = append(m.Sources, f)
		}
	}
	return nil
}

// selectSources expands a Source candidate: a directory contributes its
// C (or .s/.S) files, and any file with a platform suffix is kept only
// when it matches the target. A missing file is passed on for the C
// compiler to report.
func (m *Module) selectSources(path string, asm bool) []string {
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		if tagged, ok := m.platformFile(filepath.Base(path)); tagged && !ok {
			return nil
		}
		return []string{path}
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		ext := filepath.Ext(name)
		if e.IsDir() || (asm && ext != ".s" && ext != ".S") || (!asm && ext != ".c") {
			continue
		}
		if tagged, ok := m.platformFile(name); tagged && !ok {
			continue
		}
		out = append(out, filepath.Join(path, name))
	}
	return out
}

func byName(mf ModFile) string {
	if mf.Name == "" {
		return "an unnamed module"
	}
	return mf.Name
}

// ImportedModules lists the Requires that the module's own packages
// import, matching import paths to the longest Require name.
func (m *Module) ImportedModules(mf ModFile) map[string]bool {
	names := make([]string, 0, len(mf.Requires))
	for _, r := range mf.Requires {
		names = append(names, r.Name)
	}
	used := map[string]bool{}
	for _, p := range m.Packages {
		if p.Std || p.Module != "" {
			continue
		}
		for _, t := range p.Files {
			for _, path := range importPaths(t) {
				if owner := ownerOf(path, names); owner != "" {
					used[owner] = true
				}
			}
		}
	}
	return used
}
