package mir

import "kigumi/internal/sem"

// verifyEntities checks that what an instruction names exists and is of
// the kind the opcode reads: a function to call, a variant to build or
// test, a record type to build, a closure to capture, asm info at the
// asm node. A back end indexes these without checking.
func (p *Program) verifyEntities(f *Func) *Error {
	kindOf := func(ent sem.EntityID) sem.EntityKind {
		if int(ent) >= len(p.R.Entities) {
			return sem.EntNone
		}
		return p.R.Entity(ent).Kind
	}
	for bid, blk := range f.Blocks {
		bid := BlockID(bid)
		for i, in := range blk.Insts {
			want := ""
			switch in.Op {
			case OpCall, OpFnItem, OpCFnPtr:
				if kindOf(in.Ent) != sem.EntFn {
					want = "a function"
				}
			case OpBind:
				// The bind adapter wraps a lowered body; a std primitive
				// binds through its runtime key.
				if kindOf(in.Ent) != sem.EntFn || (p.ByEnt[in.Ent] == nil && !(p.R.Fn(in.Ent).Body == 0 && p.R.Entity(in.Ent).Flags&sem.EfStd != 0)) {
					want = "a function with a body or a std primitive"
				}
			case OpCallValue:
				if in.Ent != 0 && kindOf(in.Ent) != sem.EntFn {
					want = "a function or nothing"
				}
			case OpVariant, OpIsVariant:
				if kindOf(in.Ent) != sem.EntVariant {
					want = "a variant"
				}
			case OpRecord:
				if kindOf(in.Ent) != sem.EntType {
					want = "a record type"
				} else if n := len(p.R.TypeDecl(in.Ent).Fields); n != len(in.Args) {
					return fail("entities", bid, i, "record of %d fields built from %d values", n, len(in.Args))
				}
			case OpIsType:
				if k := kindOf(in.Ent); k != sem.EntType && k != sem.EntAlias {
					want = "a type"
				}
			case OpClosure:
				if k := kindOf(in.Ent); !((k == sem.EntClosure && p.ByEnt[in.Ent] != nil) || (in.Str == "method" && k == sem.EntFn)) {
					want = "a lowered closure, or a method for a method value"
				}
			case OpAsm:
				if p.R.AsmOf(f.File, in.Node) == nil {
					return fail("entities", bid, i, "asm without its operand table")
				}
			case OpField, OpFieldMove, OpPayload:
				if in.Index < 0 {
					return fail("entities", bid, i, "%s index %d", in.Op, in.Index)
				}
			}
			if want != "" {
				name := ""
				if int(in.Ent) < len(p.R.Entities) {
					e := p.R.Entity(in.Ent)
					name = " (" + p.R.Packages[e.Pkg].Path + "." + e.Name + ")"
					if e.Kind == sem.EntFn && p.R.Fn(in.Ent).Owner != 0 {
						name = " (" + p.R.Packages[e.Pkg].Path + "." + p.R.Entity(p.R.Fn(in.Ent).Owner).Name + "." + e.Name + ")"
					}
				}
				return fail("entities", bid, i, "%s names entity %d%s, which is not %s", in.Op, in.Ent, name, want)
			}
		}
	}
	return nil
}
