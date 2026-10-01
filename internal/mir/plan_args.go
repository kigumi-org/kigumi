package mir

import "kigumi/internal/sem"

// inPlace reports whether argument i is lent rather than handed over: the
// `self`/`mut self` receiver of a method, the target of a field, index or
// cell store, the function value of an indirect call, and every argument
// of a runtime primitive (bodiless std function), which borrows what it
// gets and retains what it keeps. A resource's own registered `drop`
// consumes `self` even when it is a bodiless std primitive: the runtime
// rule that std primitives never release is written for `self` a callee
// merely uses, not for `self` the callee IS the destructor of.
func (p *Program) inPlace(in Inst, i int) bool {
	switch in.Op {
	case OpCallValue:
		// A witness call lends its `self` / `mut self` receiver (mir
		// consumesArg agrees), like a direct method call.
		if in.Ent != 0 && i == 1 {
			recv := p.R.Fn(in.Ent).Recv
			return recv != sem.RecvNone && recv != sem.RecvMove
		}
		return i == 0
	case OpSetField, OpSetIndex, OpCellSet, OpBoxReplace:
		return i == 0
	case OpCallC, OpAsm, OpShell:
		return true
	case OpCall:
		if p.R.Entity(in.Ent).Kind != sem.EntFn {
			return false
		}
		info := p.R.Fn(in.Ent)
		if i == 0 && p.isOwnDrop(in.Ent, info) {
			return false
		}
		if info.Body == 0 && info.Abi == "" && p.R.Entity(in.Ent).Flags&sem.EfStd != 0 {
			return true
		}
		if info.Abi != "" || info.Naked {
			return true
		}
		recv := info.Recv
		return i == 0 && recv != sem.RecvNone && recv != sem.RecvMove
	}
	return false
}

// isOwnDrop reports whether fn is the registered `drop` of its own owner
// type, the one case where a bodiless std primitive still consumes `self`.
func (p *Program) isOwnDrop(fn sem.EntityID, info *sem.FnInfo) bool {
	return info.Owner != 0 && p.R.TypeDecl(info.Owner).Drop == fn
}

// isStdPrimitive reports whether an entity is a bodiless std function, which
// only the runtime implements.
func (p *Program) IsStdPrimitive(fn sem.EntityID) bool {
	if p.R.Entity(fn).Kind != sem.EntFn {
		return false
	}
	info := p.R.Fn(fn)
	return info.Body == 0 && info.Abi == "" && p.R.Entity(fn).Flags&sem.EfStd != 0
}

// copyable is what a consuming use duplicates instead of moving: Copy
// types, a type parameter included only when a `Copy` constraint proves it.
func (p *Program) copyable(t sem.TypeID) bool {
	switch p.R.Types.Kind(t) {
	case sem.KUntyped, sem.KVar, sem.KPrim:
		return true
	case sem.KRef:
		// A borrow is an alias of the caller's object: never duplicated,
		// never released.
		return false
	}
	return p.R.IsCopy(t)
}

// mustCopy reports whether an instruction consumes a named local that the
// scope still owns: Copy values (a Copy-constrained type parameter
// included) are duplicated (or retained) so the local's drop at scope exit
// stays balanced. Move-only values, type parameters included, are
// transferred as they are.
func (p *Program) mustCopy(f *Func, in Inst, a LocalID) bool {
	l := f.Locals[a]
	if l.Ent == 0 || !p.copyable(l.Type) {
		return false
	}
	switch in.Op {
	case OpDrop, OpCopy, OpBorrow, OpField, OpFieldMove, OpIndex, OpIsVariant, OpIsType, OpPayload, OpUnbox,
		OpCellGet, OpBinary, OpUnary, OpInterp, OpBuiltin:
		return false
	}
	return true
}
