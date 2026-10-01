package interp

import (
	"bufio"
	"io"
	"math"
	"os"

	"kigumi/internal/syntax"
)

func registerMathTextTime(in *Interp) {
	m := "std/math."
	unary := map[string]func(float64) float64{"sqrt": math.Sqrt, "floor": math.Floor, "ceil": math.Ceil, "round": math.Round}
	for name, fn := range unary {
		in.def(m+name, func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) { return f64(fn(flt(a[0]))), nil })
	}
	in.def(m+"pow", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return f64(math.Pow(flt(a[0]), flt(a[1]))), nil
	})
	in.def("std/prelude.String.sliceBytes", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		s := str(a[0])
		start, end := int(deref(a[1]).(Int).V), int(deref(a[2]).(Int).V)
		if start < 0 || start > end || end > len(s) || !boundary(s, start) || !boundary(s, end) {
			return mkNone(in, fr.retType(n)), nil
		}
		return mkSome(in, fr.retType(n), Str(s[start:end])), nil
	})
}

// SetStdin replaces the process stdin the program reads.
func (in *Interp) SetStdin(r io.Reader) {
	in.stdinSrc = r
	in.stdin = bufio.NewReader(r)
}

// stdinReader is the process stdin unless SetStdin replaced it.
func (in *Interp) stdinReader() *bufio.Reader {
	if in.stdin == nil {
		in.stdinSrc = os.Stdin
		in.stdin = bufio.NewReader(in.stdinSrc)
	}
	return in.stdin
}
