package driver

import (
	"fmt"
	"io"
	"kigumi/internal/buildgraph"
	"path/filepath"
	"strings"
)

// artifactOut is where an artifact lands: `<out>/<name>` for an executable,
// `<out>/<name>.a` for a library archive.
func artifactOut(outDir string, a *buildgraph.Artifact) string {
	if a.Kind == "library" {
		return filepath.Join(outDir, a.Name+".a")
	}
	return filepath.Join(outDir, a.Name)
}

// buildArtifact loads the module for the artifact's own target (so
// platform files and a second target's compile fall out of LoadModule),
// picks the entry under its directory, merges the C sources and links of
// the artifact and of everything it uses, and links it with BuildWith.
func buildArtifact(p *BuildProgram, rg *ResolvedGraph, idx int, filePath func(int) string, outDir string, opts ExecOptions, stderr io.Writer) error {
	g := rg.Graph
	a, t := g.Artifacts[idx], rg.Targets[idx]
	m, err := LoadModule(p.Module.Root, LoadOptions{StdRoot: p.stdRoot, Target: t, Deny: a.Deny, Ceiling: a.Ceiling, ArtifactName: a.Name})
	if err != nil {
		return fmt.Errorf("artifact %s: %w", a.Name, err)
	}
	entry, err := scriptUnder(m, a.Dir)
	if err != nil {
		return fmt.Errorf("artifact %s: %w", a.Name, err)
	}
	m.Entry = entry
	m.ExplicitEntry = true
	root, _ := filepath.Abs(p.Module.Root)
	seen := map[int]bool{}
	var merge func(id int)
	merge = func(id int) {
		if seen[id] {
			return
		}
		seen[id] = true
		art := g.Artifacts[id]
		for _, f := range art.ExtraSources {
			m.Sources = append(m.Sources, filePath(f))
		}
		for _, l := range art.ExtraLinks {
			if l.Search != "" && !filepath.IsAbs(l.Search) {
				l.Search = filepath.Join(root, filepath.FromSlash(l.Search))
			}
			m.Links = append(m.Links, Link{Library: l.Library, Search: l.Search})
		}
		for _, u := range art.Uses {
			merge(u)
		}
	}
	merge(idx)
	buildOpts := BuildOptions{Target: t, CFlags: opts.CFlags, LinkFlags: a.LinkFlags, Entry: a.Entry, Reproducible: opts.Reproducible, Env: opts.Env}
	if a.LinkerScript != nil {
		buildOpts.LinkerScript = filePath(*a.LinkerScript)
	}
	ok, err := BuildWith(m, artifactOut(outDir, a), stderr, buildOpts)
	if err != nil {
		return fmt.Errorf("artifact %s: %w", a.Name, err)
	}
	if !ok {
		return fmt.Errorf("artifact %s did not build", a.Name)
	}
	return nil
}

// scriptUnder names the one script-shaped file in the package tree at dir
// (root-relative, "" for the root itself).
func scriptUnder(m *Module, dir string) (string, error) {
	var scripts []string
	for _, path := range m.Order {
		p := m.Packages[path]
		rel := m.Rel(path)
		if p.Std || p.Module != "" || !(dir == "" || rel == dir || strings.HasPrefix(rel, dir+"/")) {
			continue
		}
		for _, t := range p.Files {
			if t.HasTopLevelStatements() || t.HasExplicitMain() {
				scripts = append(scripts, t.File.Name)
			}
		}
	}
	switch len(scripts) {
	case 1:
		return scripts[0], nil
	case 0:
		return "", fmt.Errorf("no entry file under %q", dir)
	}
	return "", fmt.Errorf("several entry files under %q: %s", dir, strings.Join(scripts, ", "))
}
