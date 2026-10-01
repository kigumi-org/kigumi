package interp

import (
	"bufio"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// registerOsTimeAccel gives std/os's Stdin/Stdout/Host and std/time's now/
// sleep/monotonic their Go counterparts, split out of registerHostAccel
// (the std/fs half).
func registerOsTimeAccel(in *Interp) {
	o := "std/os."
	in.hostAccel(o+"Stdin.readAll", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		b, err := io.ReadAll(in.stdinReader())
		switch {
		case err != nil:
			return mkErr(in, fr.retType(n), err.Error()), nil
		case !utf8.Valid(b):
			return mkErr(in, fr.retType(n), "invalid UTF-8"), nil
		}
		return mkOk(in, fr.retType(n), Str(b)), nil
	})
	in.hostAccel(o+"Stdin.readLine", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		optType, _, _ := in.r.Types.IsResult(fr.retType(n))
		line, err := in.stdinReader().ReadString('\n')
		if err != nil && err != io.EOF {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		if line == "" {
			return mkOk(in, fr.retType(n), mkNone(in, optType)), nil
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if !utf8.ValidString(trimmed) {
			return mkErr(in, fr.retType(n), "invalid UTF-8"), nil
		}
		return mkOk(in, fr.retType(n), mkSome(in, optType, Str(trimmed))), nil
	})
	in.hostAccel(o+"Host.env", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		if v, ok := os.LookupEnv(str(a[1])); ok && utf8.ValidString(v) {
			return mkSome(in, fr.retType(n), Str(v)), nil
		}
		return mkNone(in, fr.retType(n)), nil
	})
	in.hostAccel(o+"Stdout.write", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		written, err := in.stdout.Write([]byte(deref(a[1]).(Bytes)))
		if err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		return mkOk(in, fr.retType(n), usize(written)), nil
	})
	in.hostAccel(o+"Stdin.read", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		max := deref(a[2]).(Int).V
		if max < 0 {
			return mkErr(in, fr.retType(n), "out of memory"), nil
		}
		buf := make([]byte, max)
		got, err := in.stdinReader().Read(buf)
		if err != nil && err != io.EOF {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		arr := deref(a[1]).(*Array)
		for _, b := range buf[:got] {
			arr.Elems = append(arr.Elems, Int{V: int64(b), T: sem.TyU8})
		}
		return mkOk(in, fr.retType(n), usize(got)), nil
	})
	in.hostAccel(o+"Stdin.readSome", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		b, err := in.readSome(deref(a[1]).(Int).V)
		if err != nil {
			if err == io.EOF {
				return mkErr(in, fr.retType(n), "stdin is closed"), nil
			}
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		return mkOk(in, fr.retType(n), Bytes(b)), nil
	})
	c := "std/time."
	in.hostAccel(c+"now", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return &Record{Type: fr.retType(n), Fields: []Value{Int{V: time.Now().UnixMilli(), T: sem.TyI64}}}, nil
	})
	in.hostAccel(c+"sleep", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		time.Sleep(time.Duration(deref(a[0]).(Int).V) * time.Millisecond)
		return Unit{}, nil
	})
	in.hostAccel(c+"monotonic", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return &Record{Type: fr.retType(n), Fields: []Value{Int{V: time.Since(timeMonotonicStart).Milliseconds(), T: sem.TyI64}}}, nil
	})
	var _ *bufio.Reader
}

// timeMonotonicStart anchors time.monotonic(): time.Since keeps using the
// monotonic reading Go's time.Now carries, immune to wall-clock adjustments.
var timeMonotonicStart = time.Now()
