package vm

import (
	"strings"

	"kigumi/internal/mir"
)

// exec runs one instruction the way llgen lowers it: arguments a consumer
// takes from a still-owned local are duplicated first, then the operation
// follows the C runtime's retain rules.
func (m *Machine) exec(fr *frame, in *mir.Inst) {
	f := fr.f
	args := make([]*obj, len(in.Args))
	for i, a := range in.Args {
		args[i] = fr.locals[a]
	}
	var res *obj
	switch in.Op {
	case mir.OpNop:
		return
	case mir.OpConst:
		res = m.constant(in)
		m.chargeMemory(res)
	case mir.OpUnit:
		res = unitObj
	case mir.OpAlias:
		res = args[0]
	case mir.OpShare:
		if in.Copy {
			res = m.copy(args[0])
			m.chargeMemory(res)
		} else {
			res = retain(args[0])
		}
	case mir.OpRelease:
		m.release(args[0])
		return
	case mir.OpMove:
		res = args[0]
		if f.Locals[in.Args[0]].Ent != 0 {
			fr.locals[in.Args[0]] = nil
		}
	case mir.OpCall:
		m.site, m.siteFn = in, f
		res = m.callEntity(in, args)
	case mir.OpCallValue:
		res = m.callValue(args[0], args[1:])
	case mir.OpBuiltin:
		res = m.builtin(in.Str, args)
		m.chargeMemory(res)
	case mir.OpBinary:
		res = m.binop(in.Str, args[0], args[1])
	case mir.OpUnary:
		if in.Str == "cast" {
			res = m.cast(args[0], m.numKind(in.Type))
		} else {
			res = m.unop(in.Str, args[0])
		}
	case mir.OpRecord:
		res = mkRecord(in.Ent, args)
		m.chargeMemory(res)
	case mir.OpVariant:
		res = mkVariant(in.Ent, args)
		m.chargeMemory(res)
	case mir.OpField:
		res = m.field(args[0], in.Index)
	case mir.OpFieldMove:
		res = m.fieldMove(args[0], in.Index)
	case mir.OpSetField:
		r := deref(args[0])
		old := r.fields[in.Index]
		r.fields[in.Index] = args[1]
		m.release(old)
		return
	case mir.OpArray:
		res = mkArray(args)
		m.chargeMemory(res)
	case mir.OpIndex:
		res = m.index(args[0], args[1])
	case mir.OpSetIndex:
		m.setIndex(args[0], args[1], args[2])
		return
	case mir.OpPayload:
		res = retain(deref(args[0]).fields[in.Index])
	case mir.OpIsVariant:
		v := deref(args[0])
		res = mkBool(v.k == kVariant && v.ent == in.Ent)
	case mir.OpIsType:
		v := deref(args[0])
		res = mkBool(v.k == kBox && m.typeEnt(v.dyn) == in.Ent)
	case mir.OpUnbox:
		v := args[0]
		if v.k == kBox {
			v = retain(v.inner)
		}
		res = v
	case mir.OpBox:
		v := args[0]
		if v.k != kBox {
			b := newObj(kBox)
			b.dyn, b.inner = in.Type, v
			v = b
			m.chargeMemory(b)
		}
		res = v
	case mir.OpClosure, mir.OpFnItem, mir.OpBind:
		res = m.closure(in, args)
		m.chargeMemory(res)
	case mir.OpBorrow:
		res = args[0]
	case mir.OpNewCell:
		res = mkCell(args[0])
		m.chargeMemory(res)
	case mir.OpCellGet:
		res = retain(args[0].inner)
	case mir.OpCellSet:
		old := args[0].inner
		args[0].inner = args[1]
		m.release(old)
		return
	case mir.OpBoxReplace:
		m.boxReplace(deref(args[0]), deref(args[1]))
		return
	case mir.OpInterp:
		var sb strings.Builder
		ai := 0
		for _, piece := range in.Strs {
			if piece == "" {
				m.displayInto(&sb, args[ai])
				ai++
				continue
			}
			sb.WriteString(piece)
		}
		res = mkStr([]byte(sb.String()))
		m.chargeMemory(res)
	case mir.OpShell:
		res = m.shellPlan(in, args)
	case mir.OpDrop:
		m.release(args[0])
		if f.Locals[in.Args[0]].Ent != 0 {
			fr.locals[in.Args[0]] = nil
		}
		return
	case mir.OpPanic:
		m.abort(in.Str)
	default:
		panic(&Unsupported{What: in.Op.String()})
	}
	fr.locals[in.Dst] = res
}
