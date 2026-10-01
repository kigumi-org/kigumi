package sem

import (
	"math/big"

	"kigumi/internal/syntax"
)

type TypeDeclInfo struct {
	Form     TypeForm
	Metadata []MetaRef
	Params   []EntityID
	Fields   []EntityID
	Variants []EntityID
	Impls    []ImplAssert
	Layout   string
	// Align is layout(C, align: N)'s N, 0 when unset.
	Align   int64
	Members map[string]OverloadSetID
	Ops     map[string]OverloadSetID
	Drop    EntityID
	Home    PackageID
	Size    sizeState
}

type FnInfo struct {
	Recv       RecvKind
	Metadata   []MetaRef
	Owner      EntityID
	TypeParams []EntityID
	Params     []EntityID
	SelfParam  EntityID
	Sig        TypeID
	Declared   Effects
	Summary    EffectSummary
	Edges      []EffectEdge
	Body       syntax.NodeID
	Abi        string
	// Export names the C ABI a function with a body is exported under.
	Export string
	// Naked marks an export(C, naked) function: its body is one asm block
	// emitted as module-level assembly, and its value is a C pointer.
	Naked bool
	Set   OverloadSetID
	// RecvParams are the method's own copies of the owner's type
	// parameters when the receiver binders carry bounds (`Array[T: Eq]`).
	RecvParams []EntityID
	// Witnesses are the hidden trailing parameters carrying the interface
	// witnesses of constrained type parameters (design: witness passing).
	Witnesses []WitnessSlot
	// ConstParams are the hidden trailing parameters carrying the runtime
	// value of every const generic parameter, own or inherited from the
	// receiver's type: erased type arguments carry no such value,
	// so each becomes an ordinary implicit argument after Witnesses.
	ConstParams []EntityID
	// CalledParams are the fn-typed parameters the body calls; callers add
	// the effects of the matching arguments.
	CalledParams []EntityID
	// CalledFieldParams are `pure?` fields read through a parameter
	// (`app.view(...)`); callers add the argument's field effect.
	CalledFieldParams []FieldParamKey
	// CalledWitnessReqs are `pure?` interface requirements read through one
	// of the function's own type parameters (`t.view(x)` where `t: T`);
	// callers add the effect of the resolved witness for their type
	// argument.
	CalledWitnessReqs []WitnessReqKey
	// EffectPoly marks a `pure? fn` interface requirement.
	EffectPoly bool
	// RetLifetime is the lifetime parameter entity the return type's `'a`
	// tag names (explicit or elided), 0 when the return carries none.
	RetLifetime EntityID
}

type InterfaceInfo struct {
	Params        []EntityID
	Reqs          []EntityID
	SelfParam     EntityID
	ObjectSafe    int8
	ObjectSafeWhy string
}

type constKind uint8

const (
	constNone constKind = iota
	constInt
	constFloat
	constString
	constBytes
	constBool
	constChar
)

// constValue holds a compile-time value; exactly one field is set per Kind.
type constValue struct {
	Kind  constKind
	Int   *big.Int
	Float *big.Float
	Str   string
	Bool  bool
	Char  rune
}

const (
	constUneval uint8 = iota
	constEvaluating
	constDone
	constFailed
)

type ConstInfo struct {
	Value    constValue
	State    uint8
	Metadata []MetaRef
}

type LocalInfo struct {
	Scope    ScopeID
	Place    PlaceID
	Metadata []MetaRef
	// Lifetime is the parameter entity a parameter's own `&'a T` tag names,
	// 0 for an untagged reference or a non-reference parameter.
	Lifetime EntityID
}

type CaptureMode uint8

const (
	capCopy CaptureMode = iota
	capRetain
	capMove
	capCell
	// capBorrow lends a move-only local to a lambda passed straight to a
	// call; the callee is trusted not to keep the closure.
	capBorrow
)

type Capture struct {
	Local EntityID
	Mode  CaptureMode
}

type ClosureInfo struct {
	Params   []EntityID
	Ret      TypeID
	Sig      TypeID
	Captures []Capture
	Summary  EffectSummary
	Edges    []EffectEdge
	Copy     bool
	// Immediate marks a lambda written as a call argument: its borrowed
	// captures live only for that call.
	Immediate bool
}
