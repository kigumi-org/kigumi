package vm

import (
	"strings"

	"kigumi/internal/hostshell"
	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

// stdFn finds a std function by package path and name.
func (m *Machine) stdFn(pkg, name string) sem.EntityID {
	return m.r.PackageMember(m.r.PackageByPath(pkg), name)
}

// callStd runs a std function with a Kigumi body on lent arguments.
func (m *Machine) callStd(pkg, name string, args []*obj) *obj {
	fn := m.stdFn(pkg, name)
	if f, ok := m.p.ByEnt[fn]; ok {
		return m.call(f, args)
	}
	return m.std(m.stdKey(fn), args)
}

// shellPlan lowers a shell literal the way llgen does: std/shell.planNew
// and planPush build the Plan from Item variants.
func (m *Machine) shellPlan(in *mir.Inst, args []*obj) *obj {
	item := m.r.PackageMember(m.r.PackageByPath("std/shell"), "Item")
	variant := func(name string, payload []*obj) *obj {
		return mkVariant(m.variantOf(item, name), payload)
	}
	plan := m.callStd("std/shell", "planNew", nil)
	ai := 0
	for _, s := range in.Strs {
		var it *obj
		switch {
		case s == "|":
			it = variant("Pipe", nil)
		case s == " ":
			it = variant("Word", nil)
		case s == "$":
			it = variant("Text", []*obj{mkStr([]byte(m.display(args[ai])))})
			ai++
		case s[0] == 'r':
			op, fd, _ := strings.Cut(s[1:], ":")
			it = variant("Redirect", []*obj{mkInt(atoi(op), 64|256), mkInt(atoi(fd), 64|256)})
		default:
			it = variant("Text", []*obj{mkStr([]byte(s[1:]))})
		}
		plan = m.callStd("std/shell", "planPush", []*obj{plan, it})
	}
	return plan
}

func atoi(s string) int64 {
	var n int64
	for _, c := range s {
		n = n*10 + int64(c-'0')
	}
	return n
}

// planOf turns a Plan record into the host plan by asking std/shell.stages
// for the stage records, so word joining lives in one place.
func (m *Machine) planOf(v *obj) *hostshell.Plan {
	stages := m.callStd("std/shell", "stages", []*obj{v})
	defer m.release(stages)
	p := &hostshell.Plan{}
	for _, sv := range deref(stages).fields {
		rec := deref(sv)
		var st hostshell.Stage
		for _, a := range deref(rec.fields[0]).fields {
			st.Argv = append(st.Argv, string(deref(a).s))
		}
		for _, r := range deref(rec.fields[1]).fields {
			rr := deref(r)
			st.Redirs = append(st.Redirs, hostshell.Redirect{Op: uint32(deref(rr.fields[0]).i), Fd: int(deref(rr.fields[1]).i), Target: string(deref(rr.fields[2]).s)})
		}
		if g := deref(rec.fields[2]); len(g.fields) == 1 {
			st.Filter = hostshell.GrepFilter(string(deref(g.fields[0]).s))
		}
		p.Stages = append(p.Stages, st)
	}
	return p
}

func (m *Machine) registerShell() {
	h := m.host
	h["os.Host.shell"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj { return mkOpaque("Shell", nil) }
	h["shell.capture"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		out, errOut, _, err := hostshell.Run(m.planOf(a[0]), m.opts.Cwd)
		if err != nil {
			return m.errMsg(err.Error())
		}
		captured := m.r.Types.Node(m.resultPayload(fn)).Ent
		return m.ok(mkRecord(captured, []*obj{mkBytes(out), mkBytes(errOut)}))
	}
	h["shell.run"] = func(m *Machine, fn sem.EntityID, a []*obj) *obj {
		out, errOut, statuses, err := hostshell.Run(m.planOf(a[0]), m.opts.Cwd)
		if err != nil {
			return m.errMsg(err.Error())
		}
		m.out.Write(out)
		m.err.Write(errOut)
		codes := mkArray(nil)
		for _, st := range statuses {
			codes.fields = append(codes.fields, mkInt(int64(st), 64|256))
		}
		result := m.r.Types.Node(m.resultPayload(fn)).Ent
		return m.ok(mkRecord(result, []*obj{codes}))
	}
}
