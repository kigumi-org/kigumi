package vm

import (
	"fmt"
	"io"
	"math/big"
	"strings"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

// The step, depth and memory limits bound a sandboxed comptime evaluation:
// they fail a non-terminating, too deeply recursive or too memory-hungry
// block with a diagnostic instead of hanging the compiler, overflowing the
// Go stack or exhausting host RAM. Steps and bytes rather than wall clock
// or live RSS, so the same source fails the same way on every machine.
// examples/comptime needs ~28M steps for its naive fib(30); its string
// table is under 1KiB, so comptimeMemoryLimit leaves that fixture ample
// headroom while still catching runaway growth (alloc_budget.go) quickly.
const (
	comptimeStepLimit   = 200_000_000
	comptimeDepthLimit  = 3_000
	comptimeMemoryLimit = 64 * 1024 * 1024
)

// hostEffect marks the primitives a sandbox refuses: anything that reaches
// the file system, processes, the network, the clock, entropy, foreign
// memory or the dynamic loader.
func hostEffect(key string) bool {
	for _, p := range []string{"fs.", "os.", "net.", "shell.", "dl.", "ffi."} {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	switch key {
	case "time.now", "time.sleep", "time.monotonic", "random.fromClock":
		return true
	}
	return false
}

// EvalComptime runs one comptime block in the sandbox and returns the
// literal it produced.
func EvalComptime(prog *mir.Program, cf *mir.ComptimeFunc) (lit sem.Literal, err error) {
	m := newMachine(prog, Options{Sandbox: true, StepLimit: comptimeStepLimit, DepthLimit: comptimeDepthLimit, MemoryLimit: comptimeMemoryLimit}, io.Discard, io.Discard)
	var v *obj
	if cerr := m.catch(func() { v = deref(m.call(cf.Func, nil)) }); cerr != nil {
		if p, ok := cerr.(*Panic); ok {
			return lit, fmt.Errorf("%s", p.Msg)
		}
		return lit, cerr
	}
	switch v.k {
	case kInt:
		return sem.Literal{Kind: sem.LitInt, Int: big.NewInt(v.i)}, nil
	case kFloat:
		return sem.Literal{Kind: sem.LitFloat, Float: big.NewFloat(v.f)}, nil
	case kBool:
		return sem.Literal{Kind: sem.LitBool, Bool: v.b}, nil
	case kChar:
		return sem.Literal{Kind: sem.LitChar, Char: v.c}, nil
	case kStr:
		return sem.Literal{Kind: sem.LitString, Str: string(v.s)}, nil
	case kBytes:
		return sem.Literal{Kind: sem.LitBytes, Str: string(v.s)}, nil
	}
	return lit, fmt.Errorf("comptime value %s cannot be embedded as a literal", m.display(v))
}

// ComptimeError is a comptime block that failed to evaluate.
type ComptimeError struct {
	Site sem.ComptimeSite
	Err  error
}

// FoldComptime evaluates every comptime block of the program, in the order
// the checker met them (inner blocks first), and folds each result into
// the code that uses it. The failures come back per block.
func FoldComptime(prog *mir.Program) []ComptimeError {
	var errs []ComptimeError
	for _, cf := range prog.Comptime {
		lit, err := EvalComptime(prog, cf)
		if err != nil {
			errs = append(errs, ComptimeError{Site: cf.Site, Err: err})
			continue
		}
		prog.Fold(cf, lit)
	}
	return errs
}
