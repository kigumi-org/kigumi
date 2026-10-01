package vm

import (
	"path/filepath"
	"strings"

	"kigumi/internal/buildgraph"
	"kigumi/internal/sem"
)

// std/build primitives record into the graph behind the Builder value.
func (m *Machine) registerBuild() {
	h := m.host
	str := func(v *obj) string { return string(deref(v).s) }
	h["build.Builder.request"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		return mkOpaque("build.Request", m.graphOf(a[0]))
	}
	h["build.Builder.hostTarget"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		return mkOpaque("build.Target", m.graphOf(a[0]).Host)
	}
	h["build.Builder.target"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		return mkOpaque("build.Target", buildgraph.Target{Triple: str(a[1])})
	}
	h["build.Request.target"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		return mkOpaque("build.Target", m.graphOf(a[0]).Request.Target)
	}
	h["build.Request.flag"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		for _, f := range m.graphOf(a[0]).Request.Flags {
			if f == str(a[1]) {
				return mkBool(true)
			}
		}
		return mkBool(false)
	}
	h["build.Builder.framework"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		g := m.graphOf(a[0])
		for _, f := range g.Frameworks {
			if f.Name == str(a[1]) && f.Version == str(a[2]) {
				return mkOpaque("build.Framework", f.ID)
			}
		}
		g.Frameworks = append(g.Frameworks, buildgraph.FrameworkRef{ID: len(g.Frameworks), Name: str(a[1]), Version: str(a[2])})
		return mkOpaque("build.Framework", len(g.Frameworks)-1)
	}
	h["build.Builder.secret"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		g := m.graphOf(a[0])
		g.Secrets = append(g.Secrets, buildgraph.SecretRef{ID: len(g.Secrets), Name: str(a[1]), Provenance: m.provenance()})
		return mkOpaque("build.Secret", len(g.Secrets)-1)
	}
	h["build.Builder.executable"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj { return m.declareArtifact(a, "executable") }
	h["build.Builder.library"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj { return m.declareArtifact(a, "library") }
	h["build.Artifact.output"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		g, id := m.currentGraph(), m.handleOf(a[0], "build.Artifact")
		for _, f := range g.Files {
			if f.Kind == "artifactOutput" && f.Artifact == id {
				return mkOpaque("build.FilePath", f.ID)
			}
		}
		g.Files = append(g.Files, buildgraph.FileRef{ID: len(g.Files), Kind: "artifactOutput", Artifact: id, Provenance: m.provenance()})
		return mkOpaque("build.FilePath", len(g.Files)-1)
	}
	h["build.Artifact.uses"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		g, id := m.currentGraph(), m.handleOf(a[0], "build.Artifact")
		g.Artifacts[id].Uses = append(g.Artifacts[id].Uses, m.handleOf(a[1], "build.Artifact"))
		return unitObj
	}
	h["build.Artifact.addSource"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		g, id := m.currentGraph(), m.handleOf(a[0], "build.Artifact")
		g.Artifacts[id].ExtraSources = append(g.Artifacts[id].ExtraSources, m.handleOf(a[1], "build.FilePath"))
		return unitObj
	}
	h["build.Artifact.addLink"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		g, id := m.currentGraph(), m.handleOf(a[0], "build.Artifact")
		lib, search := str(a[1]), str(a[2])
		if strings.ContainsAny(lib, `/\`) {
			m.abort("link library \"" + lib + "\" is a name, not a path")
		}
		if search != "" {
			abs := search
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(g.Roots[0], filepath.FromSlash(search))
			}
			if !withinRoots(g.Roots, abs) {
				m.abort("link search directory " + search + " is outside the module, its dependencies and the build output directory")
			}
			search = abs
		}
		g.Artifacts[id].ExtraLinks = append(g.Artifacts[id].ExtraLinks, buildgraph.Link{Library: lib, Search: search})
		return unitObj
	}
	h["build.Artifact.linkerScript"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		g, id := m.currentGraph(), m.handleOf(a[0], "build.Artifact")
		file := m.handleOf(a[1], "build.FilePath")
		g.Artifacts[id].LinkerScript = &file
		return unitObj
	}
	h["build.Artifact.linkFlags"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		g, id := m.currentGraph(), m.handleOf(a[0], "build.Artifact")
		for _, e := range deref(a[1]).fields {
			g.Artifacts[id].LinkFlags = append(g.Artifacts[id].LinkFlags, str(e))
		}
		return unitObj
	}
	h["build.Artifact.entry"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		g, id := m.currentGraph(), m.handleOf(a[0], "build.Artifact")
		g.Artifacts[id].Entry = str(a[1])
		return unitObj
	}
	h["build.Artifact.availability"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		g, id := m.currentGraph(), m.handleOf(a[0], "build.Artifact")
		ceiling := str(a[2])
		if ceiling != "" {
			if _, ok := sem.LayerShortName[ceiling]; !ok {
				m.abort(`availability ceiling "` + ceiling + `" must be "core", "alloc", "sys", "tool" or ""`)
			}
			g.Artifacts[id].Ceiling = ceiling
		}
		for _, e := range deref(a[1]).fields {
			path := str(e)
			if pkg := m.r.PackageByPath(path); pkg == 0 || !m.r.Packages[pkg].Std {
				m.abort(`availability deny "` + path + `" is not a std package path`)
			}
			g.Artifacts[id].Deny = append(g.Artifacts[id].Deny, path)
		}
		return unitObj
	}
	m.registerBuildFiles()
}

func (m *Machine) declareArtifact(a []*obj, kind string) *obj {
	g := m.graphOf(a[0])
	name, dir := string(deref(a[1]).s), filepath.ToSlash(string(deref(a[2]).s))
	for _, art := range g.Artifacts {
		if art.Name == name {
			return m.errMsg("artifact " + name + " is declared twice")
		}
	}
	if !bareOutputName(name) || dir == ".." || strings.HasPrefix(dir, "../") || filepath.IsAbs(dir) || hasReservedPathComponent(dir) {
		return m.errMsg("artifact needs a bare name and a root-relative directory")
	}
	target, _ := deref(a[3]).data.(buildgraph.Target)
	art := &buildgraph.Artifact{ID: len(g.Artifacts), Name: name, Kind: kind, Dir: strings.TrimSuffix(dir, "/"), Target: target, Provenance: m.provenance()}
	g.Artifacts = append(g.Artifacts, art)
	return m.ok(mkOpaque("build.Artifact", art.ID))
}
