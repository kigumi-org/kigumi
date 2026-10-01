package vm

import (
	"bufio"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"kigumi/internal/sem"
)

// hostFn is a host primitive or accelerator: it sees the callee so it can
// build values of the declared result type.
type hostFn func(m *Machine, fn sem.EntityID, a []*obj) *obj

// registerHost supplies the capability objects, the process arguments,
// stdin, the environment and the file system, replacing the extern(C)
// bodies of std/fs and std/os the way the interpreter's accelerators do.
func (m *Machine) registerHost() {
	m.host = map[string]hostFn{}
	h := m.host
	h["os.Host.args"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj { return mkOpaque("Args", nil) }
	h["os.Host.files"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj { return mkOpaque("Files", nil) }
	h["os.Host.net"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj { return mkOpaque("Net", nil) }
	h["os.Host.stdin"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj { return mkOpaque("Stdin", nil) }
	h["os.Host.entropy"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj { return mkOpaque("Entropy", nil) }
	h["os.Args.len"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj { return mkInt(int64(len(m.opts.Args)), 64) }
	h["os.Args.get"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		i := deref(a[1]).i
		if i < 0 || i >= int64(len(m.opts.Args)) || !utf8.ValidString(m.opts.Args[i]) {
			return m.none()
		}
		return m.some(mkStr([]byte(m.opts.Args[i])))
	}
	h["os.Host.env"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		if v, ok := os.LookupEnv(string(deref(a[1]).s)); ok && utf8.ValidString(v) {
			return m.some(mkStr([]byte(v)))
		}
		return m.none()
	}
	h["os.Stdin.readAll"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		var out []byte
		buf := make([]byte, 4096)
		for {
			n, err := m.stdinReader().Read(buf)
			out = append(out, buf[:n]...)
			if err != nil {
				if err == io.EOF {
					break
				}
				return m.errMsg(err.Error())
			}
		}
		if !utf8.Valid(out) {
			return m.errMsg("invalid UTF-8")
		}
		return m.ok(mkStr(out))
	}
	h["os.Stdin.readLine"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		line, err := m.stdinReader().ReadString('\n')
		if err != nil && err != io.EOF {
			return m.errMsg(err.Error())
		}
		if line == "" {
			return m.ok(m.none())
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if !utf8.ValidString(trimmed) {
			return m.errMsg("invalid UTF-8")
		}
		return m.ok(m.some(mkStr([]byte(trimmed))))
	}
	h["os.Host.stdout"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj { return mkOpaque("Stdout", nil) }
	h["os.Stdout.write"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		n, err := m.out.Write(deref(a[1]).s)
		if err != nil {
			return m.errMsg(err.Error())
		}
		return m.ok(mkInt(int64(n), m.numKind(sem.TyUsize)))
	}
	h["os.Stdin.read"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		max := deref(a[2]).i
		if max < 0 {
			return m.errMsg("out of memory")
		}
		buf := make([]byte, max)
		got, err := m.stdinReader().Read(buf)
		if err != nil && err != io.EOF {
			return m.errMsg(err.Error())
		}
		arr := deref(a[1])
		for _, b := range buf[:got] {
			arr.fields = append(arr.fields, mkInt(int64(b), 8))
		}
		return m.ok(mkInt(int64(got), m.numKind(sem.TyUsize)))
	}
	h["os.Stdin.readSome"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		b, err := m.readSome(deref(a[1]).i)
		if err != nil {
			if err == io.EOF {
				return m.errMsg("stdin is closed")
			}
			return m.errMsg(err.Error())
		}
		return m.ok(mkBytes(b))
	}
	m.registerFiles()
}

func (m *Machine) stdinReader() *bufio.Reader {
	if m.stdin == nil {
		r := m.opts.Stdin
		if r == nil {
			r = os.Stdin
		}
		m.stdinSrc = r
		m.stdin = bufio.NewReader(r)
	}
	return m.stdin
}

func (m *Machine) resolvePath(p string) string {
	if m.opts.Cwd == "" || strings.HasPrefix(p, "/") {
		return p
	}
	return m.opts.Cwd + "/" + p
}
