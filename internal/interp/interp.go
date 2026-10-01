package interp

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// Interp is a tree-walking evaluator over checked trees: it reads the
// checker's side tables (types, uses, calls, coercions, patterns) and never
// re-resolves names.
type Interp struct {
	r       *sem.Result
	stdout  io.Writer
	cvalues []Value
	accels  map[string]builtinFn
	// required marks accelerators that stand in for bodies which call C:
	// the interpreter has no FFI, so KIGUMI_NO_ACCEL cannot switch them off.
	required map[string]bool
	// NoAccel runs the std bodies in Kigumi instead of the Go accelerators.
	NoAccel  bool
	stderr   io.Writer
	stdin    *bufio.Reader
	stdinSrc io.Reader
	args     []string
	builtins map[string]builtinFn
	cwd      string
	sigWatch map[string]chan os.Signal
	// DenySignals refuses Signals.watch instead of installing a real OS
	// handler: `kigumi test` runs in the same process as the CLI, and a
	// real os/signal.Notify there would hijack the runner's own Ctrl-C
	// handling for the rest of the invocation.
	DenySignals bool
}

type builtinFn func(in *Interp, fr *frame, call syntax.NodeID, args []Value) (Value, *ctrl)

// frame is one activation: cells by entity, deferred cleanups, and the
// tree the body lives in.
type frame struct {
	in         *Interp
	caller     *frame
	fn         sem.EntityID
	t          *syntax.Tree
	info       *sem.FileInfo
	cells      map[sem.EntityID]*Cell
	defers     []deferEntry
	scopes     [][]sem.EntityID
	tempScopes [][]Value
	loop       int
}

type deferEntry struct {
	node syntax.NodeID
	err  bool
	fr   *frame
}

type ctrlKind uint8

const (
	ctrlReturn ctrlKind = iota + 1
	ctrlBreak
	ctrlContinue
	ctrlFail
)

// ctrl is a non-local exit in flight: return, break, continue or fail.
type ctrl struct {
	kind ctrlKind
	val  Value
}

// Panic aborts the program: no unwinding, no cleanup.
type Panic struct {
	Msg  string
	File *token.File
	Pos  token.Pos
}

func (p *Panic) Error() string {
	if p.File == nil {
		return "panic: " + p.Msg
	}
	return fmt.Sprintf("panic: %s\n  at %s:%d:%d", p.Msg, p.File.Name, p.File.Line(p.Pos), p.File.Column(p.Pos))
}

func New(r *sem.Result, stdout, stderr io.Writer, args []string) *Interp {
	in := &Interp{r: r, stdout: stdout, stderr: stderr, args: args}
	in.builtins = map[string]builtinFn{}
	in.accels = map[string]builtinFn{}
	registerPrelude(in)
	registerOption(in)
	registerCollections(in)
	registerText(in)
	registerArrayExtra(in)
	registerCrypto(in)
	registerHost(in)
	registerHostExtra(in)
	registerShell(in)
	registerAccel(in)
	registerHostAccel(in)
	registerNetAccel(in)
	registerEntropyAccel(in)
	registerSignalAccel(in)
	registerError(in)
	return in
}

func (in *Interp) newFrame(fn sem.EntityID, f sem.FileID) *frame {
	t := in.r.Tree(f)
	return &frame{in: in, fn: fn, t: t, info: in.r.File(t), cells: map[sem.EntityID]*Cell{}}
}

func (fr *frame) panicAt(n syntax.NodeID, format string, args ...any) {
	panic(&Panic{Msg: fmt.Sprintf(format, args...), File: fr.t.File, Pos: fr.t.Span(n).Start})
}
