package driver

import (
	"fmt"
	"io"
	"kigumi/internal/vm"
	"os"

	"kigumi/internal/interp"
	"kigumi/internal/sem"
)

// RunOptions is what the CLI resolved for one run.
type RunOptions struct {
	// Stdin replaces the process stdin when not nil.
	Stdin io.Reader
	// NoAccel runs std bodies in Kigumi instead of the Go accelerators.
	NoAccel bool
	// Engine selects the executor: "vm" (the MIR VM, which shares the AOT
	// backend's lowering), "interp" (the tree walker), or "auto" (the
	// default: the VM when it supports every construct of the program).
	Engine string
}

// Run type-checks a loaded module and interprets its entry file.
func Run(m *Module, stdout, stderr io.Writer, args []string) (int, error) {
	return RunWith(m, RunOptions{}, stdout, stderr, args)
}

// RunInput is Run with stdin replaced when stdin is not nil.
func RunInput(m *Module, stdin io.Reader, stdout, stderr io.Writer, args []string) (int, error) {
	return RunWith(m, RunOptions{Stdin: stdin}, stdout, stderr, args)
}

// RunWith picks the executor before anything runs: "vm" fails on a
// construct the VM lacks, naming it, and "auto" sends such a program to the
// interpreter instead. Neither falls back after output has started.
func RunWith(m *Module, opts RunOptions, stdout, stderr io.Writer, args []string) (int, error) {
	if opts.Engine != "interp" {
		a, err := Analyze(m)
		if err != nil {
			return 1, err
		}
		if a.Res.HasErrors() {
			io.WriteString(stderr, a.Res.Render())
			return 1, nil
		}
		ok, why := vm.Supports(a.Prog)
		if ok {
			return runVM(a, opts, stdout, stderr, args)
		}
		if opts.Engine == "vm" {
			return 1, vmRefusalError(why)
		}
	}
	in, err := newInterp(m, opts, stdout, stderr, args)
	if in == nil {
		return 1, err
	}
	return in.RunEntry(m.Entry), nil
}

func runVM(a *Analysis, opts RunOptions, stdout, stderr io.Writer, args []string) (int, error) {
	cwd, _ := os.Getwd()
	code, err := vm.Run(a.Prog, vm.Options{Args: args, Stdin: opts.Stdin, Cwd: cwd, NoAccel: opts.NoAccel}, stdout, stderr)
	if err != nil {
		return 1, fmt.Errorf("%w (run with --engine interp)", err)
	}
	return code, nil
}

// RunTests type-checks a module loaded with tests and runs its test blocks.
func RunTests(m *Module, stdout, stderr io.Writer) (int, error) {
	return RunTestsWith(m, RunOptions{}, stdout, stderr)
}

// RunTestsWith picks the executor the way RunWith does, over the test
// blocks instead of the entry.
func RunTestsWith(m *Module, opts RunOptions, stdout, stderr io.Writer) (int, error) {
	if opts.Engine != "interp" {
		a, err := Analyze(m)
		if err != nil {
			return 1, err
		}
		if a.Res.HasErrors() {
			io.WriteString(stderr, a.Res.Render())
			return 1, nil
		}
		ok, why := vm.SupportsTests(a.Prog)
		if ok {
			cwd, _ := os.Getwd()
			failed, err := vm.RunTests(a.Prog, vm.Options{Stdin: opts.Stdin, Cwd: cwd, NoAccel: opts.NoAccel, DenySignals: true}, stdout, stderr)
			if err != nil {
				return 1, fmt.Errorf("%w (run with --engine interp)", err)
			}
			if failed > 0 {
				return 1, nil
			}
			return 0, nil
		}
		if opts.Engine == "vm" {
			return 1, vmRefusalError(why)
		}
	}
	in, err := newInterp(m, opts, stdout, stderr, nil)
	if in == nil {
		return 1, err
	}
	in.DenySignals = true
	if in.RunTests() > 0 {
		return 1, nil
	}
	return 0, nil
}

// newInterp checks the module and prepares an interpreter; nil (with the
// rendered diagnostics on stderr) when the module does not check.
func newInterp(m *Module, opts RunOptions, stdout, stderr io.Writer, args []string) (*interp.Interp, error) {
	res, err := Check(m)
	if err != nil {
		return nil, err
	}
	if res.HasErrors() {
		io.WriteString(stderr, res.Render())
		return nil, nil
	}
	in := interp.New(res, stdout, stderr, args)
	in.NoAccel = opts.NoAccel
	if opts.Stdin != nil {
		in.SetStdin(opts.Stdin)
	}
	return in, nil
}

var _ = sem.Hosted
