package interp

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

func opaque(kind string, data any) Value { return &Opaque{Kind: kind, Data: data} }

func registerHost(in *Interp) {
	o := "std/os."
	in.def(o+"Host.shell", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Shell", nil), nil
	})
	in.def(o+"Host.args", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Args", in.args), nil
	})
	in.def(o+"Host.files", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Files", nil), nil
	})
	in.def(o+"Host.net", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Net", nil), nil
	})
	in.def(o+"Host.dl", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Loader", nil), nil
	})
	in.def(o+"Host.entropy", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Entropy", nil), nil
	})
	in.def(o+"Host.signals", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Signals", nil), nil
	})
	in.def(o+"Args.get", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		args := deref(a[0]).(*Opaque).Data.([]string)
		i := int(deref(a[1]).(Int).V)
		if i < 0 || i >= len(args) || !utf8.ValidString(args[i]) {
			return mkNone(in, fr.retType(n)), nil
		}
		return mkSome(in, fr.retType(n), Str(args[i])), nil
	})
	in.def("std/net.Net.http", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("Http", nil), nil
	})
	in.hostAccel("std/net.Http.post", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		raw := string(deref(a[1]).(Str))
		if why := httpPostGuard(raw); why != "" {
			return mkErr(in, fr.retType(n), why), nil
		}
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(raw, "text/plain", bytes.NewReader([]byte(deref(a[2]).(Str))))
		if err != nil {
			return mkErr(in, fr.retType(n), err.Error()), nil
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		t, _, _ := in.r.Types.IsResult(fr.retType(n))
		return mkOk(in, fr.retType(n), &Record{Type: t, Fields: []Value{Int{V: int64(resp.StatusCode), T: sem.TyI64}}}), nil
	})
	in.def("std/alloc.Arena.create", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return mkOk(in, fr.retType(n), opaque("AllocatorHandle", nil)), nil
	})
	in.def("std/alloc.AllocatorHandle.scope", func(in *Interp, fr *frame, n syntax.NodeID, a []Value) (Value, *ctrl) {
		return opaque("AllocatorScope", nil), nil
	})
	registerShared(in)
	registerFFI(in)
	registerDL(in)
	registerTask(in)
}

func (in *Interp) resolvePath(p string) string {
	if in.cwd == "" || strings.HasPrefix(p, "/") {
		return p
	}
	return in.cwd + "/" + p
}
