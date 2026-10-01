package driver

import (
	"encoding/json"
	"fmt"
	"kigumi/internal/buildgraph"
)

// ResolvedGraph is a build graph with targets parsed, frameworks and
// tools matched against descriptors, and an execution order.
type ResolvedGraph struct {
	Graph      *buildgraph.Graph
	Targets    []Target
	Frameworks []FrameworkDescriptor
	// Order interleaves artifacts ("a<i>") and tool steps ("t<j>").
	Order []string
}

// ResolveGraph validates a graph before anything runs: every target
// parses, every library is freestanding, every framework and tool is
// registered, and the dependency edges form a DAG.
func ResolveGraph(g *buildgraph.Graph, descriptors []FrameworkDescriptor) (*ResolvedGraph, error) {
	rg := &ResolvedGraph{Graph: g}
	host := HostTarget()
	for _, a := range g.Artifacts {
		t, err := ParseTarget(a.Target.Triple, a.Target.Sys)
		if err != nil {
			return nil, fmt.Errorf("artifact %s: %w", a.Name, err)
		}
		if a.Kind == "library" && !t.Bare() {
			return nil, fmt.Errorf("artifact %s: host-target library archives need a freestanding target or --sys none", a.Name)
		}
		if (a.LinkerScript != nil || len(a.LinkFlags) != 0 || a.Entry != "") && t.Bare() {
			return nil, fmt.Errorf("artifact %s: a linker script, link flags and entry symbol need a hosted target", a.Name)
		}
		rg.Targets = append(rg.Targets, t)
	}
	for _, f := range g.Frameworks {
		if f.Name == builtinToolchainName && f.Version == "" {
			rg.Frameworks = append(rg.Frameworks, toolchainDescriptor(zigCacheDir(g)))
			continue
		}
		found := false
		for _, d := range descriptors {
			if d.Name == f.Name && d.Version == f.Version {
				rg.Frameworks = append(rg.Frameworks, d)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("framework %s %s has no descriptor (%s) in the module or its dependencies", f.Name, f.Version, FrameworkFileName)
		}
	}
	for _, s := range g.ToolSteps {
		if s.Kind == "artifact" {
			if s.Artifact < 0 || s.Artifact >= len(g.Artifacts) {
				return nil, fmt.Errorf("runArtifact: artifact %d does not exist", s.Artifact)
			}
			art, t := g.Artifacts[s.Artifact], rg.Targets[s.Artifact]
			if art.Kind != "executable" {
				return nil, fmt.Errorf("runArtifact %s: not an executable artifact (recorded by %s)", art.Name, s.Provenance.Package)
			}
			if t.Bare() || t.OS != host.OS || t.Arch != host.Arch {
				return nil, fmt.Errorf("runArtifact %s: not built for the host (recorded by %s)", art.Name, s.Provenance.Package)
			}
			continue
		}
		if _, ok := rg.Frameworks[s.Framework].Tools[s.Tool]; !ok {
			return nil, fmt.Errorf("tool %s is not registered by framework %s (recorded by %s)", s.Tool, rg.Frameworks[s.Framework].Name, s.Provenance.Package)
		}
	}
	order, err := buildOrder(g)
	if err != nil {
		return nil, err
	}
	rg.Order = order
	return rg, nil
}

// buildOrder sorts artifacts and tool steps so every producer precedes
// its consumers: artifacts before what uses them, tool steps before the
// files they output are read.
func buildOrder(g *buildgraph.Graph) ([]string, error) {
	producer := func(file int) (string, bool) {
		f := g.Files[file]
		switch f.Kind {
		case "toolOutput":
			return fmt.Sprintf("t%d", f.ProducedBy), true
		case "artifactOutput":
			return fmt.Sprintf("a%d", f.Artifact), true
		}
		return "", false
	}
	deps := map[string][]string{}
	var nodes []string
	for _, a := range g.Artifacts {
		id := fmt.Sprintf("a%d", a.ID)
		nodes = append(nodes, id)
		for _, u := range a.Uses {
			deps[id] = append(deps[id], fmt.Sprintf("a%d", u))
		}
		for _, f := range a.ExtraSources {
			if p, ok := producer(f); ok {
				deps[id] = append(deps[id], p)
			}
		}
		if a.LinkerScript != nil {
			if p, ok := producer(*a.LinkerScript); ok {
				deps[id] = append(deps[id], p)
			}
		}
	}
	for _, s := range g.ToolSteps {
		id := fmt.Sprintf("t%d", s.ID)
		nodes = append(nodes, id)
		if s.Kind == "artifact" {
			deps[id] = append(deps[id], fmt.Sprintf("a%d", s.Artifact))
		}
		for _, f := range s.Inputs {
			if p, ok := producer(f); ok {
				deps[id] = append(deps[id], p)
			}
		}
	}
	var order []string
	state := map[string]int{}
	var visit func(n string) error
	visit = func(n string) error {
		switch state[n] {
		case 2:
			return nil
		case 1:
			return fmt.Errorf("build steps depend on each other in a cycle at %s", n)
		}
		state[n] = 1
		for _, d := range deps[n] {
			if err := visit(d); err != nil {
				return err
			}
		}
		state[n] = 2
		order = append(order, n)
		return nil
	}
	for _, n := range nodes {
		if err := visit(n); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// PlanJSON is `kigumi build --plan`: the resolved graph as data, before
// any tool runs or artifact compiles.
func PlanJSON(rg *ResolvedGraph) string {
	type artifact struct {
		*buildgraph.Artifact
		Resolved Target `json:"resolved"`
	}
	plan := struct {
		Request    buildgraph.Request        `json:"request"`
		Frameworks []buildgraph.FrameworkRef `json:"frameworks"`
		Secrets    []buildgraph.SecretRef    `json:"secrets"`
		Files      []buildgraph.FileRef      `json:"files"`
		ToolSteps  []buildgraph.ToolStep     `json:"toolSteps"`
		Artifacts  []artifact                `json:"artifacts"`
		Order      []string                  `json:"order"`
	}{Request: rg.Graph.Request, Frameworks: rg.Graph.Frameworks, Secrets: rg.Graph.Secrets, Files: rg.Graph.Files, ToolSteps: rg.Graph.ToolSteps, Artifacts: []artifact{}, Order: rg.Order}
	for i, a := range rg.Graph.Artifacts {
		plan.Artifacts = append(plan.Artifacts, artifact{a, rg.Targets[i]})
	}
	out, _ := json.MarshalIndent(plan, "", "  ")
	return string(out) + "\n"
}
