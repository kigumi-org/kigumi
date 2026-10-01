package syntax

type NodeKind uint8

// Expression, pattern and shell node kinds. Declaration, statement and type
// kinds live in node_decl.go. The data layout of each kind is given by
// shapes[] in tree.go.
const (
	NoneKind NodeKind = iota

	Ident
	IntLit
	FloatLit
	CharLit
	BoolLit
	StringLit
	ByteStringLit
	ShellLit
	Paren
	Unary
	Binary
	IsExpr
	CallExpr
	WsCallExpr
	BracketExpr
	MemberExpr
	OptMemberExpr
	TryExpr
	RecordLit
	FieldInit
	Spread
	NamedArg
	TupleLit
	UnitLit
	Lambda
	Block
	IfExpr
	IfLetExpr
	MatchExpr
	MatchArm
	ForExpr
	ReturnExpr
	FailExpr
	BreakExpr
	ContinueExpr
	AwaitExpr
	ComptimeExpr
	UnsafeExpr
	AllocatorExpr
	ContractExpr
	BorrowExpr
	MoveExpr
	AsmExpr
	AsmTemplate
	AsmOperand
	AsmClobber
	AsmOptions
	AsmAbi

	PatWildcard
	PatBind
	PatLit
	PatCtor
	PatRecord
	PatOr
	PatField
	PatTuple
	PatRange

	ShellPlan
	ShellCmd
	ShellWord
	ShellText
	ShellInterp
	ShellRedirect

	exprKindEnd
)

// Modifier bits stored in "#mods" slots.
const (
	ModPure uint32 = 1 << iota
	ModNoalloc
	ModAsync
	ModUnsafe
)

// ModPureVar marks `pure?`; kept out of the low byte so a bare `Effects(mods)`
// conversion (used where mods carries only Pure/Noalloc/Async/Unsafe)
// truncates it away.
const ModPureVar uint32 = 1 << 8

// Flag bits stored in "#flags" slots.
const (
	FlagMut uint32 = 1 << iota
	FlagMove
	FlagSelf
	FlagVariadic
	// FlagNamed marks a VariantField declared as `name: Type`.
	FlagNamed
	// FlagConst marks a GenericParam declared as `const N: usize`.
	FlagConst
	// FlagLifetime marks a GenericParam declared as `'a`.
	FlagLifetime
)

// Shell redirect operators stored in ShellRedirect "#op".
const (
	RedirIn uint32 = iota + 1
	RedirOut
	RedirAppend
	RedirDupIn
	RedirDupOut
)

var exprKindNames = [...]string{
	NoneKind: "None", Ident: "Ident", IntLit: "IntLit", FloatLit: "FloatLit",
	CharLit: "CharLit", BoolLit: "BoolLit", StringLit: "StringLit",
	ByteStringLit: "ByteStringLit",
	ShellLit:      "ShellLit", Paren: "Paren", Unary: "Unary", Binary: "Binary",
	IsExpr: "Is", CallExpr: "Call", WsCallExpr: "WsCall",
	BracketExpr: "Bracket", MemberExpr: "Member", OptMemberExpr: "OptMember",
	TryExpr: "Try", RecordLit: "RecordLit", FieldInit: "FieldInit", Spread: "Spread",
	NamedArg: "NamedArg",
	TupleLit: "TupleLit", UnitLit: "UnitLit",
	Lambda: "Lambda", Block: "Block", IfExpr: "If", IfLetExpr: "IfLet",
	MatchExpr: "Match", MatchArm: "Arm", ForExpr: "For", ReturnExpr: "Return",
	FailExpr: "Fail", BreakExpr: "Break", ContinueExpr: "Continue", AwaitExpr: "Await",
	ComptimeExpr: "Comptime", UnsafeExpr: "Unsafe", AllocatorExpr: "Allocator",
	ContractExpr: "ContractExpr", BorrowExpr: "Borrow", MoveExpr: "Move",
	AsmExpr: "Asm", AsmTemplate: "AsmTemplate", AsmOperand: "AsmOperand",
	AsmClobber: "AsmClobber", AsmOptions: "AsmOptions", AsmAbi: "AsmAbi",
	PatWildcard: "PatWildcard", PatBind: "PatBind", PatLit: "PatLit",
	PatCtor: "PatCtor", PatRecord: "PatRecord", PatField: "PatField", PatOr: "PatOr",
	PatTuple: "PatTuple", PatRange: "PatRange",
	ShellPlan: "ShellPlan", ShellCmd: "ShellCmd", ShellWord: "ShellWord",
	ShellText: "ShellText", ShellInterp: "ShellInterp", ShellRedirect: "ShellRedirect",
}

func (k NodeKind) String() string {
	if k < exprKindEnd {
		return exprKindNames[k]
	}
	if k < kindEnd {
		return declKindNames[k-exprKindEnd]
	}
	return "NodeKind(?)"
}

func (k NodeKind) IsPattern() bool { return k >= PatWildcard && k <= PatRange }

func (k NodeKind) IsType() bool { return (k >= TypePath && k <= TypeTuple) || k == Paren }
