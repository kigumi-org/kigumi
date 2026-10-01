package driver

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExecOptions is what the CLI resolved for one build: secret files by
// declared name and the extra C flags for the artifacts.
type ExecOptions struct {
	Secrets map[string]string
	CFlags  []string
	// ToolEnv is the whole environment of a tool process:
	// nothing of the driver's own environment is inherited, so the CLI
	// passes an allowlist such as PATH and the locale.
	ToolEnv []string
	// Reproducible builds every artifact with BuildOptions.Reproducible,
	// for the --reproducible flag.
	Reproducible bool
	// Env is the allowlisted compiler environment (shared.CCEnv) each
	// artifact's Reproducible build runs its two internal builds in.
	Env []string
}

func ExecuteGraph(p *BuildProgram, rg *ResolvedGraph, outDir string, opts ExecOptions, stderr io.Writer) error {
	outDir, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	root, _ := filepath.Abs(p.Module.Root)
	roots, err := p.Roots(outDir)
	if err != nil {
		return err
	}
	g := rg.Graph
	for _, f := range g.Files {
		// Re-check literal files at execution: the tree may have changed
		// since configure ran, and a symlink must not lead outside.
		if f.Kind == "literal" && !withinSandbox(roots, filepath.Join(root, filepath.FromSlash(f.Path))) {
			return fmt.Errorf("file %s is outside the module, its dependencies and the build output directory", f.Path)
		}
	}
	filePath := func(id int) string {
		f := g.Files[id]
		switch f.Kind {
		case "literal":
			return filepath.Join(root, filepath.FromSlash(f.Path))
		case "artifactOutput":
			return artifactOut(outDir, g.Artifacts[f.Artifact])
		}
		return filepath.Join(outDir, f.Path)
	}
	for _, f := range g.Files {
		if f.Kind == "generated" {
			if err := writeGraphOutput(filePath(f.ID), []byte(f.Content), 0o644); err != nil {
				return err
			}
		}
	}
	for _, node := range rg.Order {
		var idx int
		fmt.Sscanf(node[1:], "%d", &idx)
		if node[0] == 't' {
			if err := runToolStep(rg, idx, filePath, opts, outDir, stderr); err != nil {
				return err
			}
			continue
		}
		if err := buildArtifact(p, rg, idx, filePath, outDir, opts, stderr); err != nil {
			return err
		}
	}
	return nil
}

// runToolStep runs one registered tool, or (Kind "artifact") an artifact
// built earlier in the same graph, with `$in:<i>`, `$out:<i>` and
// `$secret:<i>` filled in; a secret is copied to a private file for the
// duration of the run and removed afterwards.
func runToolStep(rg *ResolvedGraph, idx int, filePath func(int) string, opts ExecOptions, outDir string, stderr io.Writer) error {
	secrets := opts.Secrets
	s := rg.Graph.ToolSteps[idx]
	name := s.Tool
	var program string
	var prefix, extraEnv []string
	if s.Kind == "artifact" {
		art := rg.Graph.Artifacts[s.Artifact]
		name, program = art.Name, artifactOut(outDir, art)
	} else {
		tp, _ := rg.Frameworks[s.Framework].program(s.Tool)
		program, prefix, extraEnv = tp.Path, tp.Args, tp.Env
	}
	var secretFiles []string
	defer func() {
		for _, f := range secretFiles {
			os.Remove(f)
		}
	}()
	args := make([]string, 0, len(s.Args))
	display := make([]string, 0, len(s.Args))
	for _, a := range s.Args {
		if a != "" && (strings.Contains(a[1:], "$in:") || strings.Contains(a[1:], "$out:") || strings.Contains(a[1:], "$secret:")) {
			return fmt.Errorf("tool %s: a placeholder must be a whole argument, not part of %q", name, a)
		}
		d := a
		switch {
		case strings.HasPrefix(a, "$in:"):
			i, err := placeholderIndex(a, len(s.Inputs))
			if err != nil {
				return fmt.Errorf("tool %s: %w", name, err)
			}
			a = filePath(s.Inputs[i])
			d = a
		case strings.HasPrefix(a, "$out:"):
			i, err := placeholderIndex(a, len(s.Outputs))
			if err != nil {
				return fmt.Errorf("tool %s: %w", name, err)
			}
			a = filePath(s.Outputs[i])
			d = a
		case strings.HasPrefix(a, "$secret:"):
			i, err := placeholderIndex(a, len(s.Secrets))
			if err != nil {
				return fmt.Errorf("tool %s: %w", name, err)
			}
			secretName := rg.Graph.Secrets[s.Secrets[i]].Name
			src, ok := secrets[secretName]
			if !ok {
				return fmt.Errorf("tool %s needs secret %s; give --secret %s=<file> or KIGUMI_SECRET_%s", name, secretName, secretName, secretName)
			}
			data, err := os.ReadFile(src)
			if err != nil {
				return fmt.Errorf("secret %s: %w", secretName, err)
			}
			tmp, err := os.CreateTemp("", "kigumi-secret-")
			if err != nil {
				return err
			}
			if _, err := tmp.Write(data); err != nil {
				tmp.Close()
				os.Remove(tmp.Name())
				return fmt.Errorf("secret %s: %w", secretName, err)
			}
			if err := tmp.Close(); err != nil {
				os.Remove(tmp.Name())
				return fmt.Errorf("secret %s: %w", secretName, err)
			}
			secretFiles = append(secretFiles, tmp.Name())
			a = tmp.Name()
			d = "$secret:" + secretName
		}
		args = append(args, a)
		display = append(display, d)
	}
	for _, o := range s.Outputs {
		if err := checkOutputSlot(filePath(o)); err != nil {
			return fmt.Errorf("tool %s: %w", name, err)
		}
	}
	fullArgs := append(append([]string{}, prefix...), args...)
	fullDisplay := append(append([]string{}, prefix...), display...)
	cmd := exec.Command(program, fullArgs...)
	cmd.Dir = outDir
	cmd.Env = append(toolEnv(opts.ToolEnv, rg.Graph.Request.Target), extraEnv...)
	cmd.Stderr = stderr
	cmd.Stdout = stderr
	fmt.Fprintf(stderr, "build: tool %s runs trusted (no sandbox), env %s\n", name, envNames(cmd.Env))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tool %s (%s %s) failed: %w", name, program, strings.Join(fullDisplay, " "), err)
	}
	for _, o := range s.Outputs {
		p := filePath(o)
		fi, err := os.Lstat(p)
		if err != nil || !fi.Mode().IsRegular() {
			return fmt.Errorf("tool %s did not produce a regular file at %s", name, rg.Graph.Files[o].Path)
		}
	}
	return nil
}
