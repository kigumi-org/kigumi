package mir

import "fmt"

// Error is a verifier finding: a rule the program breaks, at one point of
// one function. Every rule has a negative test in verify_test.go.
type Error struct {
	Func string
	Rule string
	At   string
	Msg  string
}

func (e *Error) Error() string {
	if e.At == "" {
		return fmt.Sprintf("%s: %s: %s", e.Func, e.Rule, e.Msg)
	}
	return fmt.Sprintf("%s: %s: %s: %s", e.Func, e.Rule, e.At, e.Msg)
}

// Verify checks every function, including the comptime blocks, against
// the MIR contract: structure (block and local references in range, one
// terminator per block), instruction shape (arity and fields per opcode),
// the entities named, types, definition before use, and exactly-once
// ownership of move-only locals. The first finding is returned.
func (p *Program) Verify() error {
	for _, f := range p.allFuncs() {
		if err := p.verifyFunc(f); err != nil {
			return err
		}
	}
	return nil
}

func (p *Program) allFuncs() []*Func {
	fs := append([]*Func{}, p.Funcs...)
	for _, cf := range p.Comptime {
		fs = append(fs, cf.Func)
	}
	return fs
}

func (p *Program) verifyFunc(f *Func) error {
	for _, rule := range []func(*Func) *Error{verifyStructure, verifyShape, p.verifyEntities, p.verifyTypes, verifyDefs, p.verifyOwnership, p.verifyCellCaptures} {
		if err := rule(f); err != nil {
			err.Func = f.Name
			return err
		}
	}
	return nil
}

func at(bid BlockID, i int) string { return fmt.Sprintf("b%d/%d", bid, i) }

func fail(rule string, bid BlockID, i int, format string, args ...any) *Error {
	return &Error{Rule: rule, At: at(bid, i), Msg: fmt.Sprintf(format, args...)}
}

// verifyStructure checks that every reference points at a local or a
// block that exists and that every block ends in exactly one terminator.
func verifyStructure(f *Func) *Error {
	nb, nl := len(f.Blocks), len(f.Locals)
	for _, prm := range f.Params {
		if int(prm) >= nl {
			return &Error{Rule: "structure", Msg: fmt.Sprintf("parameter %d out of range", prm)}
		}
	}
	for bid, blk := range f.Blocks {
		bid := BlockID(bid)
		for i, in := range blk.Insts {
			if int(in.Dst) >= nl {
				return fail("structure", bid, i, "destination %d out of range", in.Dst)
			}
			for _, a := range in.Args {
				if int(a) >= nl {
					return fail("structure", bid, i, "argument %d out of range", a)
				}
			}
		}
		term := len(blk.Insts)
		switch blk.Term.Op {
		case TermNone:
			return fail("structure", bid, term, "missing terminator")
		case TermJump:
			if len(blk.Term.Targets) != 1 || len(blk.Term.Args) != 0 {
				return fail("structure", bid, term, "jump takes one target and no argument")
			}
		case TermBranch:
			if len(blk.Term.Targets) != 2 || len(blk.Term.Args) != 1 {
				return fail("structure", bid, term, "branch takes two targets and one argument")
			}
		case TermReturn:
			if len(blk.Term.Targets) != 0 || len(blk.Term.Args) != 1 {
				return fail("structure", bid, term, "return takes one argument and no target")
			}
		case TermUnreachable:
			if len(blk.Term.Targets) != 0 || len(blk.Term.Args) != 0 {
				return fail("structure", bid, term, "unreachable takes nothing")
			}
		default:
			return fail("structure", bid, term, "unknown terminator %d", blk.Term.Op)
		}
		for _, t := range blk.Term.Targets {
			if int(t) >= nb {
				return fail("structure", bid, term, "target b%d out of range", t)
			}
		}
		for _, a := range blk.Term.Args {
			if int(a) >= nl {
				return fail("structure", bid, term, "terminator argument %d out of range", a)
			}
		}
	}
	return nil
}
