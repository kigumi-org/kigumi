package vm

import (
	"fmt"
	"io"

	"kigumi/internal/buildgraph"
	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

const buildStepLimit = 2_000_000_000

// EvalBuild runs configure(b) in the sandbox and returns the graph it
// recorded. A failed Result or a panic becomes the error.
func EvalBuild(prog *mir.Program, configure sem.EntityID, req buildgraph.Request, roots buildgraph.Roots) (*buildgraph.Graph, error) {
	m := newMachine(prog, Options{Sandbox: true, StepLimit: buildStepLimit, DepthLimit: comptimeDepthLimit}, io.Discard, io.Discard)
	g := &buildgraph.Graph{Request: req, Artifacts: []*buildgraph.Artifact{}, Roots: roots.Roots, Host: roots.Host}
	m.graph = g
	var failure string
	cerr := m.catch(func() {
		res := deref(m.call(prog.ByEnt[configure], []*obj{mkOpaque("build.Builder", g)}))
		if res != nil && res.k == kVariant && res.ent == m.variantOf(m.r.Types.ResultEnt(), "Err") {
			failure = m.display(m.errorMessage(res.fields[0]))
		}
	})
	if p, ok := cerr.(*Panic); ok {
		return nil, fmt.Errorf("%s", p.Msg)
	}
	if cerr != nil {
		return nil, cerr
	}
	if failure != "" {
		return nil, fmt.Errorf("configure failed: %s", failure)
	}
	return g, nil
}

// provenance is the module and package of the nearest caller outside
// std, so a std/build helper records who asked for the step.
func (m *Machine) provenance() buildgraph.Provenance {
	for i := len(m.stack) - 1; i >= 0; i-- {
		f := m.stack[i]
		if f.Ent == 0 {
			continue
		}
		pkg := m.r.Packages[m.r.Entity(f.Ent).Pkg]
		if !pkg.Std {
			return buildgraph.Provenance{Module: pkg.Module, Package: pkg.Path}
		}
	}
	return buildgraph.Provenance{}
}

func (m *Machine) currentGraph() *buildgraph.Graph {
	if m.graph == nil {
		m.abort("std/build is only available to a build program run by `kigumi build`")
	}
	return m.graph
}

// graphOf checks that a Builder value is this build program's capability.
func (m *Machine) graphOf(v *obj) *buildgraph.Graph {
	g := m.currentGraph()
	if d := deref(v); d.k != kOpaque || d.data != any(g) {
		m.abort("not the build capability of this build program")
	}
	return g
}

func (m *Machine) handleOf(v *obj, kind string) int {
	d := deref(v)
	if d.k != kOpaque || d.op != kind {
		m.abort(fmt.Sprintf("expected a %s handle, got %s", kind, m.display(d)))
	}
	return d.data.(int)
}
