package driver

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"kigumi/internal/diag"
	"kigumi/internal/token"
)

// FrameworkDescriptor is a kigumi.framework.kg: which Framework a
// directory provides and the tools it registers. Only tools
// listed here may run; the build program never names a program path.
//
//	Framework { name: "acme/sdk", version: "1.0" }
//	Tool { name: "gen", program: "tools/gen.sh" }
type FrameworkDescriptor struct {
	Name, Version string
	Dir           string
	Tools         map[string]ToolProgram
}

// ToolProgram is what a registered tool actually runs: an executable and,
// for a tool that is really a subcommand (`zig cc`), the fixed arguments
// that select it, ahead of the step's own args. Env adds to the trusted
// allowlist (toolEnv), for a tool that needs more than PATH and the target
// to do anything at all.
type ToolProgram struct {
	Path string
	Args []string
	Env  []string
}

// builtinToolchainName is the Framework `Builder.toolchain` records:
// ResolveGraph answers it from the compiler
// the driver itself links with, instead of a kigumi.framework.kg.
const builtinToolchainName = "toolchain"

// toolchainDescriptor exposes BuildWith's own C compiler as tool "cc" and,
// when the compiler is zig, its LLD wrapper as tool "ld". zig needs a cache
// directory but the tool env allowlist has no HOME, so its tools get one
// under the build's own output directory.
func toolchainDescriptor(cacheDir string) FrameworkDescriptor {
	fd := FrameworkDescriptor{Name: builtinToolchainName, Tools: map[string]ToolProgram{}}
	cc := CCompiler()
	if len(cc) == 0 {
		return fd
	}
	var env []string
	if filepath.Base(cc[0]) == "zig" && cacheDir != "" {
		env = []string{"ZIG_GLOBAL_CACHE_DIR=" + cacheDir}
	}
	fd.Tools["cc"] = ToolProgram{Path: cc[0], Args: cc[1:], Env: env}
	if filepath.Base(cc[0]) == "zig" {
		fd.Tools["ld"] = ToolProgram{Path: cc[0], Args: []string{"ld.lld"}, Env: env}
	} else if ld, err := exec.LookPath("ld.lld"); err == nil {
		fd.Tools["ld"] = ToolProgram{Path: ld}
	} else if ld, err := exec.LookPath("ld"); err == nil {
		fd.Tools["ld"] = ToolProgram{Path: ld}
	}
	return fd
}

var frameworkFields = map[string][]string{
	"Framework": {"name", "version"},
	"Tool":      {"name", "program"},
}

// LoadFrameworks reads the descriptor of every root that has one.
func LoadFrameworks(roots []string) ([]FrameworkDescriptor, error) {
	var out []FrameworkDescriptor
	for _, dir := range roots {
		src, ok, err := readFileMaybe(filepath.Join(dir, FrameworkFileName))
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		f := token.NewFile(FrameworkFileName, src)
		recs, ds := parseRecords(f, frameworkFields)
		fd := FrameworkDescriptor{Dir: dir, Tools: map[string]ToolProgram{}}
		for _, r := range recs {
			switch r.Kind {
			case "Framework":
				fd.Name, fd.Version = r.Fields["name"], r.Fields["version"]
			case "Tool":
				if r.Fields["program"] == "" {
					ds = append(ds, diag.Errorf(diag.Location{Span: r.Span}, "Tool needs a program"))
				}
				fd.Tools[r.Fields["name"]] = ToolProgram{Path: r.Fields["program"]}
			}
		}
		if fd.Name == "" && len(ds) == 0 {
			ds = append(ds, diag.Errorf(diag.Location{Span: token.Span{}}, "the descriptor needs a Framework record"))
		}
		if len(ds) > 0 {
			return nil, fmt.Errorf("%s: %s", filepath.Join(dir, FrameworkFileName), diag.RenderAll(f, ds[:1]))
		}
		out = append(out, fd)
	}
	return out, nil
}

// program resolves a registered tool to the ToolProgram to execute; a
// relative Path is taken from the descriptor's own directory.
func (fd FrameworkDescriptor) program(tool string) (ToolProgram, bool) {
	tp, ok := fd.Tools[tool]
	if !ok || filepath.IsAbs(tp.Path) {
		return tp, ok
	}
	tp.Path = filepath.Join(fd.Dir, filepath.FromSlash(tp.Path))
	return tp, true
}
