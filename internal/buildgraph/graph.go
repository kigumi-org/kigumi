// Package buildgraph is the graph a build program records: pure data,
// no execution. Slices keep
// declaration order so --plan and link order are stable; the interpreter
// fills it and the driver performs every effect.
package buildgraph

type Target struct {
	Triple string `json:"triple,omitempty"`
	Sys    string `json:"sys,omitempty"`
}

// Provenance names the module and package whose code recorded a node.
type Provenance struct {
	Module  string `json:"module,omitempty"`
	Package string `json:"package,omitempty"`
}

type FileRef struct {
	ID         int        `json:"id"`
	Kind       string     `json:"kind"` // literal, generated, toolOutput, artifactOutput
	Path       string     `json:"path,omitempty"`
	Content    string     `json:"-"`
	ProducedBy int        `json:"producedBy,omitempty"`
	Artifact   int        `json:"artifact,omitempty"`
	Provenance Provenance `json:"provenance"`
}

type FrameworkRef struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type SecretRef struct {
	ID         int        `json:"id"`
	Name       string     `json:"name"`
	Provenance Provenance `json:"provenance"`
}

type ToolStep struct {
	ID   int    `json:"id"`
	Kind string `json:"kind,omitempty"` // "" (a Framework tool, the default) or "artifact"
	// Framework and Tool name the step when Kind is "". Artifact names it
	// when Kind is "artifact": the step runs that artifact's own output.
	Framework  int        `json:"framework,omitempty"`
	Tool       string     `json:"tool,omitempty"`
	Artifact   int        `json:"artifact,omitempty"`
	Args       []string   `json:"args,omitempty"`
	Inputs     []int      `json:"inputs,omitempty"`
	Outputs    []int      `json:"outputs"`
	Secrets    []int      `json:"secrets,omitempty"`
	Provenance Provenance `json:"provenance"`
}

type Link struct {
	Library string `json:"library,omitempty"`
	Search  string `json:"search,omitempty"`
}

type Artifact struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"` // executable, library
	Dir          string `json:"dir"`
	Target       Target `json:"target"`
	Uses         []int  `json:"uses,omitempty"`
	ExtraSources []int  `json:"extraSources,omitempty"`
	ExtraLinks   []Link `json:"extraLinks,omitempty"`
	// LinkerScript, LinkFlags and Entry configure this artifact's own link
	// step; they are not inherited through Uses.
	LinkerScript *int     `json:"linkerScript,omitempty"`
	LinkFlags    []string `json:"linkFlags,omitempty"`
	Entry        string   `json:"entry,omitempty"`
	// Deny and Ceiling are Artifact.availability's declaration
	// Deny bars exact std import paths,
	// Ceiling caps std packages by layer short name, "" for no ceiling.
	Deny       []string   `json:"deny,omitempty"`
	Ceiling    string     `json:"ceiling,omitempty"`
	Provenance Provenance `json:"provenance"`
}

type Request struct {
	Target Target   `json:"target"`
	Flags  []string `json:"flags,omitempty"`
}

type Graph struct {
	Request    Request        `json:"request"`
	Frameworks []FrameworkRef `json:"frameworks,omitempty"`
	Secrets    []SecretRef    `json:"secrets,omitempty"`
	Files      []FileRef      `json:"files,omitempty"`
	ToolSteps  []ToolStep     `json:"toolSteps,omitempty"`
	Artifacts  []*Artifact    `json:"artifacts"`
	// Roots bound Builder.file: the module root, dependency directories,
	// the output directory (absolute).
	Roots []string `json:"-"`
	Host  Target   `json:"-"`
}

// Roots is what the sandboxed build program may name files under.
type Roots struct {
	Roots []string
	Host  Target
}
