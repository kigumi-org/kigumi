package vm

import (
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"kigumi/internal/sem"
)

func pathText(v *obj) string { return string(deref(deref(v).fields[0]).s) }

func fileOf(v *obj) *os.File {
	fh, _ := deref(deref(v).fields[0]).data.(*os.File)
	return fh
}

// resultPayload is the type inside the callee's `T!` result.
func (m *Machine) resultPayload(fn sem.EntityID) sem.TypeID {
	t, _, _ := m.r.Types.IsResult(m.r.Types.Node(m.r.Fn(fn).Sig).Elem)
	return t
}

func (m *Machine) registerFiles() {
	h := m.host
	openAs := func(m *Machine, fn sem.EntityID, fh *os.File, err error) *obj {
		if err != nil {
			return m.errMsg(err.Error())
		}
		file := m.r.Types.Node(m.resultPayload(fn)).Ent
		return m.ok(mkRecord(file, []*obj{mkOpaque("Ptr", fh)}))
	}
	h["fs.Files.create"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		fh, err := os.Create(m.resolvePath(pathText(a[1])))
		return openAs(m, fn, fh, err)
	}
	h["fs.Files.append"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		fh, err := os.OpenFile(m.resolvePath(pathText(a[1])), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		return openAs(m, fn, fh, err)
	}
	h["fs.Files.remove"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		if err := os.Remove(m.resolvePath(pathText(a[1]))); err != nil {
			return m.errMsg(err.Error())
		}
		return m.ok(unitObj)
	}
	h["fs.File.write"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		fh := fileOf(a[0])
		if fh == nil {
			return m.errMsg("file is closed")
		}
		n, err := fh.Write(deref(a[1]).s)
		if err != nil {
			return m.errMsg(err.Error())
		}
		return m.ok(mkInt(int64(n), m.numKind(sem.TyUsize)))
	}
	h["fs.File.close"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		if fh := fileOf(a[0]); fh != nil {
			if err := fh.Sync(); err != nil && !strings.Contains(err.Error(), "invalid argument") {
				return m.errMsg(err.Error())
			}
		}
		return m.ok(unitObj)
	}
	h["fs.File.drop"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		if fh := fileOf(a[0]); fh != nil {
			fh.Close()
		}
		return unitObj
	}
	h["fs.Files.read"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		b, err := os.ReadFile(m.resolvePath(pathText(a[1])))
		switch {
		case err != nil:
			return m.errMsg(err.Error())
		case !utf8.Valid(b):
			return m.errMsg("invalid UTF-8")
		}
		return m.ok(mkStr(b))
	}
	h["fs.Files.readBytes"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		b, err := os.ReadFile(m.resolvePath(pathText(a[1])))
		if err != nil {
			return m.errMsg(err.Error())
		}
		return m.ok(mkBytes(b))
	}
	h["fs.Files.mkdir"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		if err := os.MkdirAll(m.resolvePath(pathText(a[1])), 0o755); err != nil {
			return m.errMsg(err.Error())
		}
		return m.ok(unitObj)
	}
	h["fs.Files.exists"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		_, err := os.Stat(m.resolvePath(pathText(a[1])))
		return mkBool(err == nil)
	}
	h["fs.Files.list"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		dir := pathText(a[1])
		entries, err := os.ReadDir(m.resolvePath(dir))
		if err != nil {
			return m.errMsg(err.Error())
		}
		var names []string
		for _, e := range entries {
			names = append(names, utf8Lossy(e.Name()))
		}
		sort.Strings(names)
		if dir != "" && !strings.HasSuffix(dir, "/") {
			dir += "/"
		}
		path := m.r.Types.Node(m.r.Types.Node(m.resultPayload(fn)).Args[0]).Ent
		out := mkArray(nil)
		for _, name := range names {
			out.fields = append(out.fields, mkRecord(path, []*obj{mkStr([]byte(dir + name))}))
		}
		return m.ok(out)
	}
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
