// Package hir is the typed semantic IR between the checked tree and MIR:
// every node carries the type, location, resolved entity,
// field index, call form and coercion the checker decided, so lowering
// reads no names and no syntax slots.
package hir

import (
	"kigumi/internal/diag"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

type ExprKind uint8

const (
	Lit         ExprKind = iota + 1 // Lit of Type
	Local                           // Ent (a local or parameter)
	FnItem                          // Ent
	Variant                         // Ent(Args...) or a nullary variant
	Host                            // the `host` intrinsic value
	Unary                           // Name Args[0]; Ent set for a user operator
	Binary                          // Args[0] Name Args[1]; Ent set for a user operator
	And                             // Args[0] && Args[1]
	Fallback                        // Args[0] || Args[1]; Ent is the branch witness; Param binds the miss value for a lambda
	Pipe                            // Args[0] |> Args[1], a lambda applied to the left operand
	Call                            // Ent(Args...) with Passes; Async wraps it in a future
	MethodCall                      // Ent with receiver Args[0]; Witness for a type-parameter receiver
	StaticCall                      // Witness(Args...)
	ValueCall                       // Args[0](Args[1:]...)
	Intrinsic                       // Name(Args...) runtime builtin
	Index                           // Args[0][Args[1]]
	Field                           // Args[0].Index
	FieldMove                       // move Args[0].Index; clears the field's slot in Args[0]
	MethodValue                     // Args[0].Ent as a closure
	OptField                        // Args[0]?.Index
	Try                             // Args[0]?; BoxType boxes the error
	Record                          // Ent { Entries }
	Lambda                          // closure Ent with body Fn
	BlockExpr                       // Block
	If                              // Cond ? Then : Else
	IfLet                           // if Pat = Cond [if Guard] Then else Else
	Match                           // match Args[0] { Arms }
	Loop                            // for [Pat in] Cond { Block }
	Return                          // Args[0] when present
	Fail                            // fail Args[0]
	Break
	Continue
	Is        // Args[0] is Pat
	Borrow    // &Args[0], Mut for &mut
	Allocator // allocator Args[0] { Block }
	Asm       // inline asm over Args; the backend reads sem.AsmInfo at Node
	Await     // await Args[0]
	Interp    // Strs with "" for each of Args
	Shell     // shell plan words in Strs, Args for interpolations
	Unit
	Wrap     // Args[0] through parentheses, `unsafe` or a spread
	Comptime // comptime { Args[0] }, not yet folded to a literal
	Panic    // Name is the message; a construct the checker let through
)

var exprNames = [...]string{Lit: "lit", Local: "local", FnItem: "fnitem", Variant: "variant", Host: "host", Unary: "un", Binary: "bin", And: "and", Fallback: "or", Pipe: "pipe", Call: "call", MethodCall: "mcall", StaticCall: "scall", ValueCall: "callv", Intrinsic: "builtin", Index: "index", Field: "field", FieldMove: "fieldmove", MethodValue: "mvalue", OptField: "optfield", Try: "try", Record: "record", Lambda: "lambda", BlockExpr: "block", If: "if", IfLet: "iflet", Match: "match", Loop: "loop", Return: "return", Fail: "fail", Break: "break", Continue: "continue", Is: "is", Borrow: "borrow", Allocator: "allocator", Asm: "asm", Await: "await", Interp: "interp", Shell: "shell", Unit: "unit", Wrap: "wrap", Comptime: "comptime", Panic: "panic"}

func (k ExprKind) String() string { return exprNames[k] }

type Expr struct {
	Kind ExprKind
	Type sem.TypeID
	Loc  diag.Location
	Node syntax.NodeID
	Lit  sem.Literal
	// Ent is the local read, the function or variant called, the record
	// type built or the closure entity.
	Ent sem.EntityID
	// Name is the operator text, the intrinsic name or the panic message.
	Name string
	Args []*Expr
	Cond *Expr
	Then *Block
	// Else is an expression: a block or a chained `if`.
	Else  *Expr
	Block *Block
	Strs  []string
	// Index is the field, payload or place index.
	Index int
	Mut   bool
	// Async marks a call of an async function, which yields a future.
	Async bool
	// Variadic packs the trailing arguments into an array.
	Variadic bool
	Witness  sem.EntityID
	Passes   []sem.WitnessArg
	// ConstArgs are the hidden const-parameter arguments a call to a
	// function with const parameters hands over after Passes.
	ConstArgs []sem.ConstArg
	// BoxType is the concrete error type a `?` boxes, 0 for none.
	BoxType sem.TypeID
	Pat     *Pat
	Guard   *Expr
	Arms    []*Arm
	Entries []*RecordEntry
	Param   *LocalDecl
	Fn      *Func
	Coercion
}

// Coercion lists the checker's coercion steps applied to a value: From is
// the boxed value's type, Ent the function a C pointer is taken of.
type Coercion struct {
	Coerce []sem.CoStep
	CoFrom sem.TypeID
	CoEnt  sem.EntityID
}

type Arm struct {
	Pat   *Pat
	Guard *Expr
	Body  *Expr
}

// RecordEntry sets field Index from Value; a Spread copies the listed
// fields out of Value, then consumes it.
type RecordEntry struct {
	Node   syntax.NodeID
	Index  int
	Value  *Expr
	Spread bool
	Fields []SpreadField
}

type SpreadField struct {
	Index, SrcIndex int
	Type            sem.TypeID
}
