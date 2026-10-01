// Package mir lowers checked trees to a block-structured IR with mutable
// locals (SSA is left to LLVM's mem2reg), runs the ownership analysis and
// inserts drops.
package mir

import (
	"kigumi/internal/diag"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

type LocalID uint32
type BlockID uint32

type Opcode uint8

const (
	OpConst      Opcode = iota + 1 // Dst = Lit (Type)
	OpUnit                         // Dst = ()
	OpCopy                         // Dst = Args[0]
	OpMove                         // Dst = Args[0], source consumed
	OpCall                         // Dst = Ent(Args...)
	OpCallValue                    // Dst = Args[0](Args[1:]...)
	OpBuiltin                      // Dst = Str(Args...) runtime function
	OpBinary                       // Dst = Args[0] Str Args[1]
	OpUnary                        // Dst = Str Args[0]
	OpRecord                       // Dst = Ent { Args... }
	OpVariant                      // Dst = Ent(Args...)
	OpField                        // Dst = Args[0].Index
	OpFieldMove                    // Dst = Args[0].Index, then Args[0].Index is cleared
	OpSetField                     // Args[0].Index = Args[1]
	OpArray                        // Dst = [Args...]
	OpIndex                        // Dst = Args[0][Args[1]]
	OpSetIndex                     // Args[0][Args[1]] = Args[2]
	OpTag                          // Dst = variant tag of Args[0]
	OpPayload                      // Dst = payload Index of Args[0]
	OpIsVariant                    // Dst = Args[0] is variant Ent
	OpIsType                       // Dst = existential Args[0] holds Ent
	OpUnbox                        // Dst = value inside existential Args[0]
	OpBox                          // Dst = existential over Args[0] (Type = dynamic type)
	OpClosure                      // Dst = closure Ent capturing Args...
	OpFnItem                       // Dst = function Ent
	OpBind                         // Dst = function Ent with its trailing witness parameters bound to Args
	OpCFnPtr                       // Dst = C function pointer to Ent
	OpCallC                        // Dst = Args[0](Args[1:]...) through a C function pointer of Type
	OpAsm                          // Dst = inline asm at Node over input Args (sem.AsmInfo)
	OpBorrow                       // Dst = &Args[0] (Index: 1 for mut)
	OpNewCell                      // Dst = cell holding Args[0]
	OpCellGet                      // Dst = *Args[0]
	OpCellSet                      // *Args[0] = Args[1]
	OpInterp                       // Dst = string of Args... (Strs holds text pieces)
	OpShell                        // Dst = plan (Strs/Args describe words)
	OpDrop                         // drop Args[0]
	OpAlias                        // Dst = Args[0], the same reference: no runtime call
	OpShare                        // Dst = a new reference to Args[0] (Copy: rt_copy vs rt_retain)
	OpRelease                      // release the temporary Args[0]
	OpPanic                        // abort with Str
	OpNop                          // removed instruction
	OpBoxReplace                   // Args[0]'s boxed value becomes Args[1]'s, in place; the old value ends up owned by Args[1] and is dropped
)

// Inst is one instruction; the fields used depend on Op.
type Inst struct {
	Op    Opcode
	Dst   LocalID
	Args  []LocalID
	Type  sem.TypeID
	Ent   sem.EntityID
	Str   string
	Strs  []string
	Lit   sem.Literal
	Index int
	Node  syntax.NodeID
	// Share makes an OpCopy take its own reference: the source is a
	// temporary some scope still drops on its own.
	Share bool
	// Copy says an OpShare takes its reference via rt_copy (a Copy type)
	// rather than rt_retain.
	Copy bool
	// Async marks an OpFnItem/OpClosure for an async fn/method value
	// (A25-b): calling it builds a Future instead of running the body.
	Async bool
}

type TermOp uint8

const (
	TermNone TermOp = iota
	TermJump
	TermBranch // Args[0] ? Targets[0] : Targets[1]
	TermReturn // Args[0]
	TermUnreachable
)

type Term struct {
	Op      TermOp
	Args    []LocalID
	Targets []BlockID
}

type Block struct {
	Insts []Inst
	Term  Term
}

type Local struct {
	Name string
	Type sem.TypeID
	Ent  sem.EntityID
	Cell bool
	// Borrowed marks a `self` / `mut self` receiver: an alias of the
	// caller's value that the method neither owns nor may move out.
	Borrowed bool
}

// Func is one lowered body: a user function, a closure, an implicit main
// or a test.
type Func struct {
	Name   string
	Ent    sem.EntityID
	File   sem.FileID
	Params []LocalID
	Ret    sem.TypeID
	Locals []Local
	Blocks []Block
	Diags  []diag.Diagnostic
	// ImplicitOk marks a script's implicit main: falling off its statement
	// list returns a bare Unit standing for Ok(()), unlike every other
	// return of a fallible-Unit function, which must carry a real Ok.
	ImplicitOk bool
}

type Program struct {
	R     *sem.Result
	Funcs []*Func
	ByEnt map[sem.EntityID]*Func
	Entry *Func
	// Comptime lists the comptime blocks, each lowered as a function of its
	// own that the driver evaluates and folds before anything runs.
	Comptime []*ComptimeFunc
}

var opNames = [...]string{
	OpConst: "const", OpUnit: "unit", OpCopy: "copy", OpMove: "move", OpCall: "call",
	OpCallValue: "callv", OpBuiltin: "builtin", OpBinary: "bin", OpUnary: "un",
	OpRecord: "record", OpVariant: "variant", OpField: "field", OpFieldMove: "fieldmove", OpSetField: "setfield",
	OpArray: "array", OpIndex: "index", OpSetIndex: "setindex", OpTag: "tag",
	OpPayload: "payload", OpIsVariant: "isvariant", OpIsType: "istype", OpUnbox: "unbox",
	OpBox: "box", OpClosure: "closure", OpFnItem: "fnitem", OpBind: "bind", OpCFnPtr: "cfnptr", OpCallC: "callc", OpAsm: "asm", OpBorrow: "borrow",
	OpNewCell: "newcell", OpCellGet: "cellget", OpCellSet: "cellset", OpInterp: "interp",
	OpShell: "shell", OpDrop: "drop", OpAlias: "alias", OpShare: "share", OpRelease: "release", OpPanic: "panic", OpNop: "nop",
	OpBoxReplace: "boxreplace",
}

func (o Opcode) String() string { return opNames[o] }
