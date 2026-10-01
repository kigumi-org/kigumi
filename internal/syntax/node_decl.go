package syntax

const (
	File NodeKind = exprKindEnd + iota
	FnDecl
	Param
	GenericParam
	TypeDecl
	RecordBody
	ResourceBody
	AdtBody
	AliasBody
	Field
	Variant
	VariantField
	Metadata
	MetaItem
	InterfaceDecl
	ConstDecl
	ImportDecl
	ImportAlias
	TestDecl
	AbiBlock
	ContractBlock
	InstantiateDecl
	LayoutClause
	Attr
	Visibility

	LetStmt
	ExprStmt
	AssignStmt
	DeferStmt
	ErrdeferStmt

	List

	TypePath
	// TypeLifetime is a `'a` used as a generic type argument;
	// as a reference type's own lifetime tag it lives on TypeRef instead.
	TypeLifetime
	TypeOptional
	TypeResult
	TypeFn
	TypePtr
	TypeRef
	TypeTuple
	Path

	kindEnd
)

var declKindNames = [...]string{
	"File", "FnDecl", "Param", "GenericParam", "TypeDecl", "RecordBody",
	"ResourceBody", "AdtBody", "AliasBody", "Field", "Variant", "VariantField",
	"Metadata", "MetaItem", "InterfaceDecl", "ConstDecl", "ImportDecl", "ImportAlias", "TestDecl",
	"AbiBlock", "ContractBlock", "Instantiate", "Layout", "Attr", "Visibility",
	"Let", "ExprStmt", "Assign", "Defer", "Errdefer",
	"List",
	"TypePath", "TypeLifetime", "TypeOptional", "TypeResult", "TypeFn", "TypePtr", "TypeRef", "TypeTuple", "Path",
}

// shape says how a node's Tok/Lhs/Rhs are to be read.
type shape uint8

const (
	sLeaf  shape = iota // Tok only
	sL                  // Lhs is a child node
	sLR                 // Lhs and Rhs are child nodes
	sList               // Extra[Lhs:Rhs] are child nodes
	sRec                // Extra[Lhs:Rhs] are named slots, see recFields
	sPath               // Extra[Lhs:Rhs] are token indices
	sFlagR              // Lhs is a flag word, Rhs a child node
	sTokR               // Lhs is a token index, Rhs a child node
	sSpan               // Lhs and Rhs are byte offsets
)

// Slot names for sRec kinds. A leading '#' marks a flag/int slot and '@' a
// token index; other slots are child nodes.
var recFields = map[NodeKind][]string{
	FnDecl:        {"@doc", "attrs", "#mods", "vis", "recv", "@name", "generics", "params", "ret", "body"},
	Param:         {"attrs", "#flags", "type"},
	TypeDecl:      {"@doc", "attrs", "vis", "@name", "generics", "layout", "body"},
	Field:         {"vis", "#flags", "type", "metadata"},
	InterfaceDecl: {"@doc", "attrs", "vis", "@name", "generics", "members"},
	ConstDecl:     {"@doc", "attrs", "vis", "@name", "type", "value"},
	ImportDecl:    {"vis", "binding", "path"},
	LetStmt:       {"#flags", "pattern", "type", "init", "else"},
	IfExpr:        {"cond", "then", "else"},
	IfLetExpr:     {"#flags", "pattern", "init", "guard", "then", "else"},
	MatchArm:      {"pattern", "guard", "body"},
	ForExpr:       {"pattern", "head", "body"},
	TypeFn:        {"#mods", "@abi", "params", "#flags", "ret"},
	AsmOperand:    {"@label", "@kind", "@reg", "expr"},
	ShellRedirect: {"#op", "#fd", "target"},
}

var shapes = map[NodeKind]shape{
	Ident: sLeaf, IntLit: sLeaf, FloatLit: sLeaf, CharLit: sLeaf, BoolLit: sLeaf,
	StringLit: sL, ByteStringLit: sLeaf, ShellLit: sTokR, Paren: sL, Unary: sL, Binary: sLR,
	IsExpr: sLR, CallExpr: sLR, WsCallExpr: sLR, BracketExpr: sLR,
	MemberExpr: sL, OptMemberExpr: sL, TryExpr: sL, RecordLit: sLR, FieldInit: sL,
	Spread: sL, NamedArg: sL, TupleLit: sList, UnitLit: sLeaf, Lambda: sLR, Block: sList, IfExpr: sRec, IfLetExpr: sRec, MatchExpr: sLR,
	MatchArm: sRec, ForExpr: sRec, ReturnExpr: sL, FailExpr: sL, BreakExpr: sL,
	ContinueExpr: sL, AwaitExpr: sL, ComptimeExpr: sL, UnsafeExpr: sL,
	AllocatorExpr: sLR, ContractExpr: sFlagR, BorrowExpr: sFlagR, MoveExpr: sL,
	AsmExpr: sL, AsmTemplate: sL, AsmOperand: sRec, AsmClobber: sL, AsmOptions: sL, AsmAbi: sLeaf,
	PatWildcard: sLeaf, PatBind: sFlagR, PatLit: sL, PatCtor: sLR,
	PatRecord: sLR, PatField: sL, PatOr: sList, PatTuple: sList, PatRange: sLR,
	ShellPlan: sList, ShellCmd: sList, ShellWord: sList, ShellText: sSpan,
	ShellInterp: sL, ShellRedirect: sRec,
	File: sList, FnDecl: sRec, Param: sRec, GenericParam: sFlagR, TypeDecl: sRec,
	RecordBody: sList, ResourceBody: sList, AdtBody: sList, AliasBody: sL, Field: sRec,
	Variant: sL, VariantField: sFlagR, Metadata: sList, MetaItem: sLR, InterfaceDecl: sRec,
	ConstDecl: sRec, ImportDecl: sRec, ImportAlias: sLR, TestDecl: sTokR, AbiBlock: sLR, ContractBlock: sFlagR,
	InstantiateDecl: sTokR, LayoutClause: sL, Attr: sLR, Visibility: sTokR,
	LetStmt: sRec, ExprStmt: sL, AssignStmt: sLR, DeferStmt: sL, ErrdeferStmt: sL,
	List:     sList,
	TypePath: sLR, TypeLifetime: sLeaf, TypeOptional: sL, TypeResult: sL, TypeFn: sRec, TypePtr: sFlagR,
	TypeRef: sFlagR, TypeTuple: sList, Path: sPath,
}
