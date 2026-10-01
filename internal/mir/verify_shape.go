package mir

import "strconv"

// shape is the operand contract of one opcode: how many arguments it
// takes (max -1 for any number), whether it yields a value, and which
// fields it must carry.
type shape struct {
	min, max int
	result   bool
	ent      bool
	typ      bool
	str      bool
}

var shapes = map[Opcode]shape{
	OpConst:      {0, 0, true, false, true, false},
	OpUnit:       {0, 0, true, false, false, false},
	OpCopy:       {1, 1, true, false, false, false},
	OpMove:       {1, 1, true, false, false, false},
	OpCall:       {0, -1, true, true, false, false},
	OpCallValue:  {1, -1, true, false, false, false},
	OpBuiltin:    {0, -1, true, false, false, true},
	OpBinary:     {2, 2, true, false, false, true},
	OpUnary:      {1, 1, true, false, false, true},
	OpRecord:     {0, -1, true, true, false, false},
	OpVariant:    {0, -1, true, true, true, false},
	OpField:      {1, 1, true, false, false, false},
	OpFieldMove:  {1, 1, true, false, false, false},
	OpSetField:   {2, 2, false, false, false, false},
	OpArray:      {0, -1, true, false, false, false},
	OpIndex:      {2, 2, true, false, false, false},
	OpSetIndex:   {3, 3, false, false, false, false},
	OpTag:        {1, 1, true, false, false, false},
	OpPayload:    {1, 1, true, false, false, false},
	OpIsVariant:  {1, 1, true, true, false, false},
	OpIsType:     {1, 1, true, true, false, false},
	OpUnbox:      {1, 1, true, false, false, false},
	OpBox:        {1, 1, true, false, true, false},
	OpClosure:    {0, -1, true, true, false, false},
	OpFnItem:     {0, 0, true, true, false, false},
	OpBind:       {0, -1, true, true, false, false},
	OpCFnPtr:     {0, 0, true, true, true, false},
	OpCallC:      {1, -1, true, false, true, false},
	OpAsm:        {0, -1, true, false, true, false},
	OpBorrow:     {1, 1, true, false, false, false},
	OpNewCell:    {1, 1, true, false, false, false},
	OpCellGet:    {1, 1, true, false, false, false},
	OpCellSet:    {2, 2, false, false, false, false},
	OpInterp:     {0, -1, true, false, false, false},
	OpShell:      {0, -1, true, false, false, false},
	OpDrop:       {1, 1, false, false, false, false},
	OpAlias:      {1, 1, true, false, false, false},
	OpShare:      {1, 1, true, false, false, false},
	OpRelease:    {1, 1, false, false, false, false},
	OpPanic:      {0, 0, false, false, false, true},
	OpNop:        {0, 0, false, false, false, false},
	OpBoxReplace: {2, 2, false, false, false, false},
}

// Yields reports whether an opcode defines its destination.
func (o Opcode) Yields() bool { return shapes[o].result }

// verifyShape checks each instruction against its opcode's contract.
func verifyShape(f *Func) *Error {
	for bid, blk := range f.Blocks {
		bid := BlockID(bid)
		for i, in := range blk.Insts {
			s, ok := shapes[in.Op]
			if !ok {
				return fail("shape", bid, i, "unknown opcode %d", in.Op)
			}
			if len(in.Args) < s.min || (s.max >= 0 && len(in.Args) > s.max) {
				return fail("shape", bid, i, "%s takes %s, got %d", in.Op, arity(s), len(in.Args))
			}
			if s.ent && in.Ent == 0 {
				return fail("shape", bid, i, "%s needs an entity", in.Op)
			}
			if s.typ && in.Type == 0 {
				return fail("shape", bid, i, "%s needs a type", in.Op)
			}
			if s.str && in.Str == "" {
				return fail("shape", bid, i, "%s needs its text", in.Op)
			}
			switch in.Op {
			case OpInterp:
				if n := countStr(in.Strs, ""); n != len(in.Args) {
					return fail("shape", bid, i, "interp has %d holes for %d arguments", n, len(in.Args))
				}
			case OpShell:
				if n := countStr(in.Strs, "$"); n != len(in.Args) {
					return fail("shape", bid, i, "shell has %d holes for %d arguments", n, len(in.Args))
				}
			case OpBorrow:
				if in.Index != 0 && in.Index != 1 {
					return fail("shape", bid, i, "borrow index %d is neither shared nor mut", in.Index)
				}
			case OpUnary:
				if in.Str == "cast" && in.Type == 0 {
					return fail("shape", bid, i, "cast needs its target type")
				}
			}
		}
	}
	return nil
}

func arity(s shape) string {
	switch {
	case s.max < 0:
		return "at least " + itoa(s.min) + " arguments"
	case s.min == s.max:
		return itoa(s.min) + " arguments"
	}
	return itoa(s.min) + " to " + itoa(s.max) + " arguments"
}

func itoa(n int) string { return strconv.Itoa(n) }

func countStr(ss []string, want string) int {
	n := 0
	for _, s := range ss {
		if s == want {
			n++
		}
	}
	return n
}
