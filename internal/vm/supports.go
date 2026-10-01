package vm

import (
	"strings"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

// Supports reports whether the VM can run every instruction and primitive
// reachable from the entry, naming the first gap otherwise, so the driver
// picks an executor before any output is produced. Reachability is by
// name for interface calls and includes every drop hook, which over-
// approximates but never misses a callee.
func Supports(prog *mir.Program) (bool, string) {
	return supportsFrom(prog, prog.Entry)
}

// Implements reports whether the VM has an implementation of a bodiless
// std function: a host accelerator or a Go primitive.
func Implements(prog *mir.Program, fn sem.EntityID) bool {
	m := &Machine{p: prog, r: prog.R, host: map[string]hostFn{}}
	m.registerAll()
	key := m.stdKey(fn)
	_, hosted := m.host[key]
	return hosted || m.primitiveKnown(key)
}

// SupportsTests is Supports over every test block instead of the entry.
func SupportsTests(prog *mir.Program) (bool, string) {
	return supportsFrom(prog, testFuncs(prog)...)
}

func testFuncs(prog *mir.Program) []*mir.Func {
	var fs []*mir.Func
	for _, f := range prog.Funcs {
		if prog.R.Entity(f.Ent).Kind == sem.EntTest {
			fs = append(fs, f)
		}
	}
	return fs
}

func supportsFrom(prog *mir.Program, roots ...*mir.Func) (bool, string) {
	m := &Machine{p: prog, r: prog.R, host: map[string]hostFn{}}
	m.registerAll()
	seen := map[*mir.Func]bool{}
	var work []*mir.Func
	add := func(f *mir.Func) {
		if f != nil && !seen[f] {
			seen[f] = true
			work = append(work, f)
		}
	}
	for _, f := range roots {
		add(f)
	}
	for ent, f := range prog.ByEnt {
		e := m.r.Entity(ent)
		if e.Kind == sem.EntFn && m.r.TypeDecl(e.Parent).Drop == ent {
			add(f)
		}
	}
	for len(work) > 0 {
		f := work[0]
		work = work[1:]
		for _, blk := range f.Blocks {
			for _, in := range blk.Insts {
				if why := m.supportsInst(&in); why != "" {
					return false, why
				}
				for _, callee := range m.callees(&in) {
					add(callee)
				}
			}
		}
	}
	return true, ""
}

// callees lists the functions an instruction can enter: the named function,
// or every method of that name when the call goes through an interface.
func (m *Machine) callees(in *mir.Inst) []*mir.Func {
	switch in.Op {
	case mir.OpCall, mir.OpClosure, mir.OpFnItem, mir.OpBind:
	default:
		return nil
	}
	if f, ok := m.p.ByEnt[in.Ent]; ok {
		// A host accelerator replaces the body, so what the body calls
		// (the C shims of std/fs, std/shell) never runs here.
		if _, hosted := m.host[m.stdKey(in.Ent)]; hosted {
			return nil
		}
		return []*mir.Func{f}
	}
	e := m.r.Entity(in.Ent)
	if e.Kind != sem.EntFn || m.r.Entity(e.Parent).Kind != sem.EntInterface {
		return nil
	}
	var fs []*mir.Func
	for ent, f := range m.p.ByEnt {
		if m.r.Entity(ent).Name != e.Name {
			continue
		}
		// A host accelerator replaces the body here too, the same as the
		// direct-call branch above: an accelerated implementer's body
		// (e.g. its extern(C) calls) never actually runs.
		if _, hosted := m.host[m.stdKey(ent)]; hosted {
			continue
		}
		fs = append(fs, f)
	}
	return fs
}

func (m *Machine) supportsInst(in *mir.Inst) string {
	switch in.Op {
	case mir.OpCallC, mir.OpAsm, mir.OpCFnPtr:
		return in.Op.String()
	case mir.OpCall:
		e := m.r.Entity(in.Ent)
		if e.Kind != sem.EntFn {
			return ""
		}
		info := m.r.Fn(in.Ent)
		if info.Abi != "" || info.Naked {
			return "foreign function " + e.Name
		}
		if _, lowered := m.p.ByEnt[in.Ent]; lowered || m.r.Entity(e.Parent).Kind == sem.EntInterface {
			return ""
		}
		key := m.stdKey(in.Ent)
		if _, ok := m.host[key]; ok || m.primitiveKnown(key) {
			return ""
		}
		return "primitive " + key
	case mir.OpBuiltin:
		switch in.Str {
		case "print", "eprint", "panic", "message", "iter.len", "iter.at", "iter.at_ref", "iter.at_move", "host", "alloc.push", "future", "await":
			return ""
		case "comptime":
			return "an unevaluated comptime block"
		}
		return "builtin " + in.Str
	}
	return ""
}

var knownPrimitives = map[string]bool{
	"array.Array.empty": true, "array.Array.of": true, "array.Array.push": true, "array.Array.growBy": true, "array.Array.len": true,
	"array.Array.get": true, "array.Array.with": true, "array.Array.withMut": true, "array.Array.takeAt": true, "array.Array.clone": true,
	"math.sqrt": true, "math.floor": true, "math.ceil": true, "math.round": true, "math.pow": true,
	"prelude.String.len": true, "prelude.Bytes.len": true, "prelude.String.toBytes": true, "prelude.String.bytes": true,
	"prelude.String.fromBytes": true, "prelude.String.tryFromBytes": true, "prelude.String.sliceBytes": true,
	"prelude.Bytes.fromArray": true, "prelude.Bytes.tryFromArray": true,
	"prelude.Bytes.concat": true, "prelude.Bytes.tryConcat": true,
	"prelude.Bytes.slice": true, "prelude.Bytes.zeros": true, "prelude.Bytes.fill": true,
	"prelude.Bytes.get": true, "prelude.Char.fromInt": true, "text.fromChar": true, "text.parseFloat": true,
	"prelude.debug":       true,
	"prelude.Bool.branch": true, "prelude.Option.branch": true, "prelude.Result.branch": true, "prelude.Option.take": true,
	"alloc.Arena.create": true, "alloc.AllocatorHandle.scope": true, "alloc.AllocatorHandle.allocated": true, "alloc.Shared.new": true,
	"alloc.Shared.clone": true, "alloc.Weak.clone": true, "alloc.Shared.get": true, "alloc.Shared.set": true, "alloc.Shared.with": true, "alloc.Shared.withMut": true,
	"alloc.Shared.downgrade": true, "alloc.Weak.upgrade": true,
	"crypto/subtle.constantTimeEq": true,
}

// primitiveKnown covers the table above and the key families answered by
// pattern: conversions, wrapping arithmetic and the Eq / Ord / Hash
// witnesses of the primitive types.
func (m *Machine) primitiveKnown(key string) bool {
	if knownPrimitives[key] {
		return true
	}
	if strings.HasPrefix(key, "prelude.") {
		for _, suffix := range []string{".equals", ".compareTo", ".hash"} {
			if strings.HasSuffix(key, suffix) {
				return true
			}
		}
		if strings.Contains(key, ".wrapping") || strings.Contains(key, ".rotate") {
			return true
		}
		if strings.Contains(strings.TrimPrefix(key, "prelude."), ".to") {
			return true
		}
	}
	return false
}
