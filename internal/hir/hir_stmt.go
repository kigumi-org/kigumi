package hir

import (
	"kigumi/internal/diag"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

type PatKind uint8

const (
	PatWild   PatKind = iota + 1
	PatBind           // Ent
	PatLit            // Lit
	PatCtor           // Ent(Subs...): a variant, or an existential type test with one sub
	PatRecord         // Ent { Fields }: a record or a variant with named payloads
	PatOr             // Subs alternatives
	PatRange          // Lo..Hi or Lo..=Hi (Inclusive)
	PatNever          // a pattern form the builder never matches
)

var patNames = [...]string{PatWild: "_", PatBind: "bind", PatLit: "lit", PatCtor: "ctor", PatRecord: "record", PatOr: "or", PatRange: "range", PatNever: "never"}

func (k PatKind) String() string { return patNames[k] }

type Pat struct {
	Kind   PatKind
	Type   sem.TypeID
	Node   syntax.NodeID
	Ent    sem.EntityID
	Lit    *Expr
	Subs   []*Pat
	Fields []*PatField
	// IsType marks a PatCtor that tests an existential's dynamic type;
	// Variant marks a PatRecord over a variant's named payloads.
	IsType  bool
	Variant bool
	// Borrow marks a pattern matched through a borrow; see sem.PatInfo.
	Borrow bool
	// Mut is Borrow's mutability.
	Mut bool
	// Lo and Hi bound a PatRange; Inclusive marks a `..=` high bound.
	Lo, Hi    *Expr
	Inclusive bool
}

// PatField names one field of a record pattern: Payload indexes a variant
// payload, otherwise Index is the record field; Sub matches it or Bind
// binds it whole.
type PatField struct {
	Node    syntax.NodeID
	Type    sem.TypeID
	Index   int
	Payload bool
	Sub     *Pat
	Bind    sem.EntityID
}

type StmtKind uint8

const (
	LetStmt     StmtKind = iota + 1 // let Pat = Expr [else Else]
	AssignStmt                      // Place = Expr, or Place op= Expr when OpText is set
	DiscardStmt                     // _ = Expr
	ExprStmt                        // Expr, dropped
	EvalStmt                        // Expr evaluated for its effect only
	DeferStmt                       // defer / errdefer of Body or Block
)

type Stmt struct {
	Kind StmtKind
	Loc  diag.Location
	Node syntax.NodeID
	Pat  *Pat
	Expr *Expr
	Else *Block
	// Place is an assignment's target.
	Place *Place
	// Compound marks `place op= value`: OpText is the builtin operator,
	// OpEnt the user operator function when one applies.
	Compound bool
	OpText   string
	OpEnt    sem.EntityID
	// Err marks errdefer; Body or Block is what runs on unwind.
	Err   bool
	Body  *Stmt
	Block *Block
}

type PlaceKind uint8

const (
	PlaceLocal PlaceKind = iota + 1 // Expr reads it, Ent stores
	PlaceField                      // Base.Index
	PlaceIndex                      // Base[Idx]
)

type Place struct {
	Kind  PlaceKind
	Node  syntax.NodeID
	Type  sem.TypeID
	Ent   sem.EntityID
	Expr  *Expr
	Base  *Expr
	Idx   *Expr
	Index int
}

type Block struct {
	Type  sem.TypeID
	Node  syntax.NodeID
	Stmts []*Stmt
	Tail  *Expr
	Coercion
}

type LocalDecl struct {
	Ent  sem.EntityID
	Name string
	Type sem.TypeID
	Mut  bool
	// Cell marks a capture shared by reference with the enclosing function.
	Cell bool
}

type FuncKind uint8

const (
	FuncFn FuncKind = iota + 1
	FuncClosure
	FuncMain
	FuncTest
)

// Func is one lowered body. A function's Body is its block expression;
// a script's Main holds the top-level statements.
type Func struct {
	Kind FuncKind
	Ent  sem.EntityID
	Name string
	File sem.FileID
	// Self is a method receiver: borrowed unless SelfMove, and not dropped
	// by the method when borrowed or when the method is the Destructor.
	Self       *LocalDecl
	SelfMove   bool
	Destructor bool
	Params     []*LocalDecl
	Witness    []*LocalDecl
	// Const are the hidden const-parameter locals, bound
	// after Witness the same way a call's ConstArgs follow its Passes.
	Const    []*LocalDecl
	Captures []*LocalDecl
	Ret      sem.TypeID
	Body     *Expr
	Main     *Block
}
