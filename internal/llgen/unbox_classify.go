package llgen

import (
	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

// scalarInfo is a local materialized in a native register: Ty is the
// register type, Nk the numeric-kind tag rt_int/rt_iadd/... expect
// (meaningless for TDouble, TI1 and TI32).
type scalarInfo struct {
	Ty Type
	Nk int
}

// scalarType covers Bool, Char, f64/Int and integers except i128/u128 (the
// boxed model carries only 64 bits of payload for those). f32 stays boxed:
// every boxed float rounds to single precision on construction (rt_float),
// which native ops don't replicate.
func (e *emitter) scalarType(t sem.TypeID) (Type, bool) {
	tt := e.r.Types
	switch {
	case t == sem.TyBool:
		return TI1, true
	case t == sem.TyChar:
		return TI32, true
	case t == sem.TyF64:
		return TDouble, true
	case tt.IsInteger(t) && t != sem.TyI128 && t != sem.TyU128:
		return TI64, true
	}
	return TVoid, false
}

// classifyScalars finds locals that can live in a native register for
// their whole lifetime: excluded are parameters, closure Cells, and
// borrowed receivers; the rest are disqualified unless every use is one
// classifyDst/classifyArgs/classifyTerm know how to keep unboxed.
func (e *emitter) classifyScalars(f *mir.Func) map[mir.LocalID]scalarInfo {
	out := map[mir.LocalID]scalarInfo{}
	isParam := make([]bool, len(f.Locals))
	for _, p := range f.Params {
		isParam[p] = true
	}
	for i, l := range f.Locals {
		if isParam[i] || l.Cell || l.Borrowed {
			continue
		}
		if ty, ok := e.scalarType(l.Type); ok {
			out[mir.LocalID(i)] = scalarInfo{Ty: ty, Nk: e.numKind(l.Type)}
		}
	}
	disqualify := func(l mir.LocalID) { delete(out, l) }
	for _, blk := range f.Blocks {
		for _, in := range blk.Insts {
			classifyDst(in, disqualify)
			classifyArgs(in, disqualify)
		}
		classifyTerm(blk.Term, out, disqualify)
	}
	return out
}

// classifyDst disqualifies in.Dst unless in.Op is one the scalar codegen
// computes without ever creating a box for its result.
func classifyDst(in mir.Inst, disqualify func(mir.LocalID)) {
	if !in.Op.Yields() {
		return
	}
	switch in.Op {
	case mir.OpConst, mir.OpBinary, mir.OpAlias, mir.OpShare, mir.OpMove:
	case mir.OpUnary:
		if in.Str == "cast" {
			disqualify(in.Dst)
		}
	default:
		disqualify(in.Dst)
	}
}

// classifyArgs disqualifies locals an instruction can't read natively.
// OpIndex/OpSetIndex allow the index arg boxed-on-demand; OpDrop/OpRelease
// become no-ops instead of disqualifying; everything else needs a real box.
func classifyArgs(in mir.Inst, disqualify func(mir.LocalID)) {
	switch in.Op {
	case mir.OpBorrow:
		disqualify(in.Args[0])
	case mir.OpAlias, mir.OpShare, mir.OpMove:
	case mir.OpUnary:
		if in.Str == "cast" {
			disqualify(in.Args[0])
		}
	case mir.OpBinary:
	case mir.OpIndex, mir.OpSetIndex:
		for i, a := range in.Args {
			if i != 1 {
				disqualify(a)
			}
		}
	case mir.OpDrop, mir.OpRelease:
	default:
		for _, a := range in.Args {
			disqualify(a)
		}
	}
}

// classifyTerm lets a Bool TermBranch condition read straight from its i1
// register; TermReturn always boxes on demand, crossing the C ABI.
func classifyTerm(t mir.Term, out map[mir.LocalID]scalarInfo, disqualify func(mir.LocalID)) {
	switch t.Op {
	case mir.TermBranch:
		if info, ok := out[t.Args[0]]; ok && info.Ty != TI1 {
			disqualify(t.Args[0])
		}
	}
}
