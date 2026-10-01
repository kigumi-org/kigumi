package vm

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"kigumi/internal/buildgraph"
	"kigumi/internal/diag"
	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

// Panic is a program abort with the runtime's message.
type Panic struct{ Msg string }

func (p *Panic) Error() string { return "panic: " + p.Msg }

// Unsupported reports an instruction or primitive the VM does not run yet.
type Unsupported struct{ What string }

func (u *Unsupported) Error() string { return "vm: " + u.What + " is not supported" }

// Options is what the host hands a run: the program arguments, stdin,
// the directory relative paths resolve against, and whether the Kigumi
// bodies of host-backed std functions run instead of the accelerators.
type Options struct {
	Args    []string
	Stdin   io.Reader
	Cwd     string
	NoAccel bool
	// Sandbox refuses host effects and bounds the run by StepLimit
	// instructions, DepthLimit nested calls and MemoryLimit allocated
	// bytes, so a comptime block or build program fails
	// with a diagnostic instead of hanging or exhausting RAM. MemoryLimit
	// of 0 leaves allocation unbounded (EvalBuild does not set it).
	Sandbox     bool
	StepLimit   int
	DepthLimit  int
	MemoryLimit int64
	// DenySignals refuses Signals.watch instead of installing a real OS
	// handler: `kigumi test` runs in the same process as the CLI, and a
	// real sigaction/os-signal.Notify there would hijack the runner's own
	// Ctrl-C handling for the rest of the invocation.
	DenySignals bool
}

// Machine runs one program.
type Machine struct {
	p        *mir.Program
	r        *sem.Result
	out, err io.Writer
	opts     Options
	drops    map[sem.EntityID]*mir.Func
	stdin    *bufio.Reader
	stdinSrc io.Reader
	named    map[string]sem.EntityID
	host     map[string]hostFn
	cvalues  []*obj
	// site is the call being dispatched, for primitives whose meaning
	// depends on the argument types at the call (ffi.sizeOf).
	site   *mir.Inst
	siteFn *mir.Func
	// stack is the active calls, innermost last; std/build reads the
	// nearest non-std caller from it as a step's provenance.
	stack        []*mir.Func
	steps, depth int
	mem          int64
	graph        *buildgraph.Graph
	sigWatch     map[string]chan os.Signal
}

// Run executes the program's entry and returns its exit code: 0, 1 for a
// main that fails with an Error, 2 for a panic.
func Run(prog *mir.Program, opts Options, out, errOut io.Writer) (int, error) {
	if prog.Entry == nil {
		return 1, fmt.Errorf("vm: program has no entry")
	}
	m := newMachine(prog, opts, out, errOut)
	var code int
	err := m.catch(func() {
		res := m.call(prog.Entry, nil)
		code = m.exitCode(res)
	})
	if p, ok := err.(*Panic); ok {
		fmt.Fprintf(errOut, "%s: %s\n", diag.Prefix("panic"), p.Msg)
		return 2, nil
	}
	return code, err
}

func newMachine(prog *mir.Program, opts Options, out, errOut io.Writer) *Machine {
	m := &Machine{p: prog, r: prog.R, out: out, err: errOut, opts: opts, drops: map[sem.EntityID]*mir.Func{}, named: map[string]sem.EntityID{}}
	m.registerAll()
	return m
}

// registerAll installs every host accelerator: the Go implementations that
// stand in for std bodies the VM cannot run (extern(C) hosts) or has no
// body for.
func (m *Machine) registerAll() {
	m.registerHost()
	m.registerShell()
	m.registerTask()
	m.registerFFI()
	m.registerTime()
	m.registerSignal()
	m.registerBuild()
	m.registerNet()
	m.registerEntropy()
	m.registerError()
}

// catch turns the machine's aborts (raised with panic) into errors.
func (m *Machine) catch(f func()) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			switch e := rec.(type) {
			case *Panic, *Unsupported:
				err = e.(error)
			default:
				panic(rec)
			}
		}
	}()
	f()
	return nil
}

func (m *Machine) abort(msg string) { panic(&Panic{Msg: msg}) }

func (m *Machine) exitCode(res *obj) int {
	res = deref(res)
	if res != nil && res.k == kVariant && res.ent == m.variantOf(m.r.Types.ResultEnt(), "Err") {
		fmt.Fprintf(m.err, "%s: %s\n", diag.Prefix("error"), m.display(m.errorMessage(res.fields[0])))
		return 1
	}
	return 0
}
