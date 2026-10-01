package interp

import (
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"kigumi/internal/syntax"
)

// The fs, os and time bodies in std reach the host through C, which the
// interpreter cannot call; these are their Go counterparts. Path is
// record{text}, File is resource{f} holding an *os.File.
func registerHostAccel(in *Interp) {
	f := "std/fs."
	pathText := func(v Value) string { return string(deref(deref(v).(*Record).Fields[0]).(Str)) }
	fileOf := func(v Value) *os.File {
		fh, _ := deref(deref(v).(*Record).Fields[0]).(*Opaque).Data.(*os.File)
		return fh
	}
	in.hostAccel(f+"Files.create", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		fh, err := os.Create(in.resolvePath(pathText(a[1])))
		if err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		t, _, _ := in.r.Types.IsResult(fr.retType(n))
		return mkOk(in, fr.retType(n), &Record{Type: t, Fields: []Value{opaque("Ptr", fh)}}), nil
	})
	in.hostAccel(f+"Files.append", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		fh, err := os.OpenFile(in.resolvePath(pathText(a[1])), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		t, _, _ := in.r.Types.IsResult(fr.retType(n))
		return mkOk(in, fr.retType(n), &Record{Type: t, Fields: []Value{opaque("Ptr", fh)}}), nil
	})
	in.hostAccel(f+"Files.remove", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		if err := os.Remove(in.resolvePath(pathText(a[1]))); err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		return mkOk(in, fr.retType(n), Unit{}), nil
	})
	in.hostAccel(f+"File.write", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		fh := fileOf(a[0])
		if fh == nil {
			return mkErr(in, fr.retType(n), "file is closed"), nil
		}
		written, err := fh.Write([]byte(deref(a[1]).(Bytes)))
		if err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		return mkOk(in, fr.retType(n), usize(written)), nil
	})
	in.hostAccel(f+"File.close", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		if fh := fileOf(a[0]); fh != nil {
			if err := fh.Sync(); err != nil && !strings.Contains(err.Error(), "invalid argument") {
				return mkErr(in, fr.retType(n), err.Error()), nil
			}
		}
		return mkOk(in, fr.retType(n), Unit{}), nil
	})
	in.hostAccel(f+"File.drop", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		if fh := fileOf(a[0]); fh != nil {
			fh.Close()
		}
		return Unit{}, nil
	})
	in.hostAccel(f+"Files.read", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		b, err := os.ReadFile(in.resolvePath(pathText(a[1])))
		switch {
		case err != nil:
			return mkErr(in, fr.retType(n), err.Error()), nil
		case !utf8.Valid(b):
			return mkErr(in, fr.retType(n), "invalid UTF-8"), nil
		}
		return mkOk(in, fr.retType(n), Str(b)), nil
	})
	in.hostAccel(f+"Files.readBytes", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		b, err := os.ReadFile(in.resolvePath(pathText(a[1])))
		if err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		return mkOk(in, fr.retType(n), Bytes(b)), nil
	})
	in.hostAccel(f+"Files.mkdir", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		if err := os.MkdirAll(in.resolvePath(pathText(a[1])), 0o755); err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		return mkOk(in, fr.retType(n), Unit{}), nil
	})
	in.hostAccel(f+"Files.exists", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		_, err := os.Stat(in.resolvePath(pathText(a[1])))
		return Bool(err == nil), nil
	})
	in.hostAccel(f+"Files.list", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		dir := pathText(a[1])
		entries, err := os.ReadDir(in.resolvePath(dir))
		if err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, utf8Lossy(e.Name()))
		}
		sort.Strings(names)
		if dir != "" && !strings.HasSuffix(dir, "/") {
			dir += "/"
		}
		t, _, _ := in.r.Types.IsResult(fr.retType(n))
		pathType := in.r.Types.Node(t).Args[0]
		out := &Array{}
		for _, name := range names {
			out.Elems = append(out.Elems, &Record{Type: pathType, Fields: []Value{Str(dir + name)}})
		}
		return mkOk(in, fr.retType(n), out), nil
	})
	registerOsTimeAccel(in)
}

// utf8Lossy replaces invalid UTF-8 bytes with U+FFFD like range does,
// keeping the String invariant for dirent names. Native
// mirrors this in ffi.stringLossy.
func utf8Lossy(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		b.WriteRune(r)
	}
	return b.String()
}
