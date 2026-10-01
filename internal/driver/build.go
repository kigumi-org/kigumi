package driver

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"kigumi/internal/llgen"
)

// Compile type-checks a module, lowers it to MIR and returns the LLVM IR
// text; ownership errors are rendered like checker errors.
func Compile(m *Module, stderr io.Writer) (string, bool, error) {
	out, ok, err := compile(m, stderr, llgen.EmitOptions{})
	return out.IR, ok, err
}

func compile(m *Module, stderr io.Writer, opts llgen.EmitOptions) (llgen.Output, bool, error) {
	if m.ReportDiagnostics(stderr) {
		return llgen.Output{}, false, nil
	}
	a, err := Analyze(m)
	if err != nil {
		return llgen.Output{}, false, err
	}
	if a.Res.HasErrors() {
		io.WriteString(stderr, a.Res.Render())
		return llgen.Output{}, false, nil
	}
	m.stats.Emit++
	return llgen.EmitProgram(a.Prog, opts), true, nil
}

// BuildOptions selects the target; the zero value builds for the host.
type BuildOptions struct {
	Target Target
	// CFlags are extra C compiler and linker arguments (the CLI passes
	// KIGUMI_CFLAGS; tests link sanitizers and helpers with them). Only its
	// compiler-shaped tokens reach the archive path (splitArchiveCFlags).
	CFlags []string
	// LinkerScript, LinkFlags and Entry come from a build program's
	// Artifact.linkerScript / linkFlags / entry; they apply to a hosted
	// link only, never to the archive path below.
	LinkerScript string
	LinkFlags    []string
	Entry        string
	// Reproducible builds m twice under buildReproducible instead of once,
	// for the --reproducible flag.
	Reproducible bool
	// Env, given by the CLI (shared.CCEnv), is the base environment
	// Reproducible's two internal builds run their C compiler in;
	// internal/driver may not read the ambient environment itself.
	// Unused when Reproducible is false.
	Env []string
	// pinCompDir and ccEnv are set only by buildReproducible's own two
	// nested builds, to give the hosted link the cmd.Dir/relative-input
	// treatment buildArchive always uses, plus an isolated cache. A plain
	// build leaves both zero, so a relative KIGUMI_CFLAGS path still
	// resolves against the process cwd.
	pinCompDir bool
	ccEnv      []string
}

// Build links a host executable at exe.
func Build(m *Module, exe string, stderr io.Writer) (bool, error) {
	return BuildWith(m, exe, stderr, BuildOptions{})
}

// BuildWith writes the IR and the runtime into a work directory and
// produces the artifact at out: an executable, or for a freestanding
// target a static archive whose `kigumi_main` the embedder calls.
func BuildWith(m *Module, out string, stderr io.Writer, opts BuildOptions) (bool, error) {
	if opts.Reproducible {
		return buildReproducible(m, out, stderr, opts)
	}
	return buildOnce(m, out, stderr, opts)
}

// buildOnce is BuildWith's single-build implementation; buildReproducible
// calls it twice with pinCompDir and ccEnv set.
func buildOnce(m *Module, out string, stderr io.Writer, opts BuildOptions) (bool, error) {
	t := opts.Target
	if t.OS == "" {
		t = HostTarget()
	}
	emitOpts := llgen.EmitOptions{Freestanding: t.Bare()}
	if t.OS == "darwin" {
		emitOpts.SymbolPrefix = "_"
	}
	code, ok, err := compile(m, stderr, emitOpts)
	if err != nil || !ok {
		return ok, err
	}
	cc := CCompiler()
	if cc == nil {
		return false, fmt.Errorf("no C compiler found (need zig or clang)")
	}
	dir, err := os.MkdirTemp("", "kigumi-build")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(dir)
	ll := filepath.Join(dir, "module.ll")
	if err := os.WriteFile(ll, []byte(code.IR), 0o644); err != nil {
		return false, err
	}
	inputs := []string{ll}
	if code.CShim != "" {
		shim := filepath.Join(dir, "kigumi_shim.c")
		if err := os.WriteFile(shim, []byte(code.CShim), 0o644); err != nil {
			return false, err
		}
		inputs = append(inputs, shim)
	}
	for _, f := range llgen.RuntimeFiles(t.Sys, t.OS) {
		path := filepath.Join(dir, f.Name)
		if err := os.WriteFile(path, []byte(f.Src), 0o644); err != nil {
			return false, err
		}
		if strings.HasSuffix(f.Name, ".c") {
			inputs = append(inputs, path)
		}
	}
	inputs = append(inputs, m.Sources...)
	// llgen emits no target triple, so the compiler always "overrides" one
	// from the host; only that expected warning is silenced.
	args := append(cc[1:], "-Wno-override-module", "-O1")
	if t.compileTriple() != "" {
		args = append(args, "-target", t.compileTriple())
	}
	if t.Bare() {
		compileFlags, _ := splitArchiveCFlags(opts.CFlags)
		args = append(args, compileFlags...)
		return buildArchive(cc, args, inputs, dir, out, opts.ccEnv, stderr)
	}
	linkOut, linkInputsList, err := linkInputs(opts.pinCompDir, dir, out, inputs)
	if err != nil {
		return false, err
	}
	args = append(args, "-o", linkOut)
	args = append(args, linkInputsList...)
	if opts.pinCompDir {
		args = append(args, "-fdebug-compilation-dir=.")
	}
	args = append(args, "-lm")
	if _, ok := m.Packages["std/dl"]; ok {
		args = append(args, "-ldl")
	}
	for _, l := range m.Links {
		if l.Search != "" {
			// l.Search may still be relative to the process cwd (a Link's
			// search path under a relative module root); resolve it now,
			// since -L below is otherwise read against cmd.Dir, which
			// pinCompDir moves to dir instead.
			search := l.Search
			if abs, err := filepath.Abs(search); err == nil {
				search = abs
			}
			args = append(args, "-L"+search)
		}
		if l.Library != "" {
			args = append(args, "-l"+l.Library)
		}
	}
	if opts.LinkerScript != "" {
		args = append(args, "-T", opts.LinkerScript)
	}
	if opts.Entry != "" {
		args = append(args, "-Wl,--entry="+opts.Entry)
	}
	args = append(args, opts.LinkFlags...)
	args = append(args, opts.CFlags...)
	cmd := exec.Command(cc[0], args...)
	cmd.Stderr = stderr
	if opts.pinCompDir {
		cmd.Dir = dir
	}
	if opts.ccEnv != nil {
		cmd.Env = opts.ccEnv
	}
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("link failed: %w", err)
	}
	return true, nil
}
