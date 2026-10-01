package interp

import (
	"sort"
	"strconv"
	"strings"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// Accelerators are Go fast paths for std functions that also have a Kigumi
// body; each pair must agree byte for byte (testdata/run/strings_edge).
func registerAccel(in *Interp) {
	p := "std/prelude.String."
	sub := func(a []Value) (string, string) { return str(a[0]), str(a[1]) }
	in.accel(p+"contains", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		s, x := sub(a)
		return Bool(strings.Contains(s, x)), nil
	})
	in.accel(p+"startsWith", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		s, x := sub(a)
		return Bool(strings.HasPrefix(s, x)), nil
	})
	in.accel(p+"endsWith", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		s, x := sub(a)
		return Bool(strings.HasSuffix(s, x)), nil
	})
	in.accel(p+"matchesAt", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		s, x := sub(a)
		at := uint64(deref(a[2]).(Int).V)
		return Bool(len(x) <= len(s) && at <= uint64(len(s)-len(x)) && s[at:at+uint64(len(x))] == x), nil
	})
	in.accel(p+"trim", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return Str(strings.Trim(str(a[0]), " \t\r\n")), nil
	})
	in.accel(p+"split", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		s, sep := sub(a)
		if sep == "" {
			return strArray([]string{s}), nil
		}
		return strArray(strings.Split(s, sep)), nil
	})
	in.accel(p+"repeat", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return Str(strings.Repeat(str(a[0]), int(deref(a[1]).(Int).V))), nil
	})
	in.accel(p+"toUpper", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return Str(shiftASCII(str(a[0]), 'a', 'z', -32)), nil
	})
	in.accel(p+"toLower", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return Str(shiftASCII(str(a[0]), 'A', 'Z', 32)), nil
	})
	registerTextAccel(in)
}

func registerTextAccel(in *Interp) {
	t := "std/text."
	in.accel(t+"indexOf", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		i := strings.Index(str(a[0]), string(rune(deref(a[1]).(Char))))
		if i < 0 {
			return mkNone(in, fr.retType(n)), nil
		}
		return mkSome(in, fr.retType(n), Int{V: int64(i), T: sem.TyUsize}), nil
	})
	in.accel(t+"lines", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		s := strings.TrimSuffix(str(a[0]), "\n")
		if s == "" {
			return &Array{}, nil
		}
		return strArray(strings.Split(s, "\n")), nil
	})
	in.accel(t+"replace", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		s, old := str(a[0]), str(a[1])
		if old == "" {
			return Str(s), nil
		}
		return Str(strings.ReplaceAll(s, old, str(a[2]))), nil
	})
	in.accel(t+"parseInt", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		v, err := strconv.ParseInt(strings.Trim(str(a[0]), " \t"), 10, 64)
		if err != nil {
			return mkNone(in, fr.retType(n)), nil
		}
		return mkSome(in, fr.retType(n), Int{V: v, T: sem.TyI64}), nil
	})
	in.accel("std/array.join", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		var items []string
		for _, e := range deref(a[0]).(*Array).Elems {
			items = append(items, str(e))
		}
		return Str(strings.Join(items, str(a[1]))), nil
	})
}

func shiftASCII(s string, lo, hi byte, delta int) string {
	b := []byte(s)
	for i, c := range b {
		if c >= lo && c <= hi {
			b[i] = byte(int(c) + delta)
		}
	}
	return string(b)
}

func strArray(items []string) *Array {
	out := &Array{}
	for _, s := range items {
		out.Elems = append(out.Elems, Str(s))
	}
	return out
}

func (in *Interp) accel(key string, f builtinFn) { in.accels[key] = f }

// hostAccel registers the interpreter's implementation of a std body that
// reaches the host through C; it stays on under KIGUMI_NO_ACCEL.
func (in *Interp) hostAccel(key string, f builtinFn) {
	if in.required == nil {
		in.required = map[string]bool{}
	}
	in.required[key] = true
	in.accels[key] = f
}

// AccelKeys lists the accelerated std functions, for the coverage test.
func (in *Interp) AccelKeys() []string {
	keys := make([]string, 0, len(in.accels))
	for k := range in.accels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
