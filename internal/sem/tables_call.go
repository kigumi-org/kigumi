package sem

import (
	"kigumi/internal/syntax"
	"strconv"
	"strings"

	"kigumi/internal/token"
)

type CallKind uint8

const (
	CallFn CallKind = iota + 1
	CallAssoc
	CallMethod
	CallVariant
	CallFnValue
	CallIntrinsic
	CallOperator
	CallBuiltinOp
	CallIndex
	CallIndexWitness
	CallPipe
	CallFallback
	CallTry
	CallMethodValue
	CallInterp
	CallIter
	CallIterProtocol
	// CallIterMap is a `for` over a Map[K, V]: the element is a (K, V)
	// tuple read from its keyList/valueList in lockstep.
	CallIterMap
	CallRecord
	CallStatic
)

type VariadicForm uint8

const (
	VariadicNone VariadicForm = iota
	VariadicElements
	VariadicSpread
)

type CallInfo struct {
	// Piped is the left operand of `|>`, prepended as the call's first
	// argument. Already included in ArgOrder when that is set — don't
	// prepend it again in that case.
	Piped    syntax.NodeID
	Kind     CallKind
	Callee   EntityID
	Next     EntityID
	Inst     []TypeID
	Recv     RecvKind
	Variadic VariadicForm
	Op       token.Kind
	OpText   string
	// Witness is the hidden local holding the requirement implementation
	// when the callee is a requirement on a type parameter.
	Witness EntityID
	// Passes are the witness arguments a call to a constrained generic
	// function hands over after its ordinary arguments, one per slot.
	Passes []WitnessArg
	// ConstArgs are the hidden const-parameter arguments a call hands over
	// after Passes, one per FnInfo.ConstParams slot.
	ConstArgs []ConstArg
	// ArgOrder is the call's arguments reordered to parameter position when
	// a named argument moved them out of source order; nil when already in
	// order. hir reads this instead of the raw syntax
	// children when set.
	ArgOrder []syntax.NodeID
}

type WitnessKind uint8

const (
	WitnessLocal WitnessKind = iota + 1 // the caller's own hidden local Ent
	WitnessFn                           // the function Ent itself
	WitnessBind                         // Ent with its own witnesses Args bound
)

// WitnessArg describes how a call site produces one witness value.
type WitnessArg struct {
	Kind WitnessKind
	Ent  EntityID
	Args []WitnessArg
	// ConstArgs are Ent's own const-parameter arguments, bound
	// into the closure the same way Args binds its own witnesses: a
	// WitnessBind's callee can need both.
	ConstArgs []ConstArg
}

type PatKind uint8

const (
	PatWild PatKind = iota
	PatBinding
	PatLiteral
	PatVariant
	PatRecordTy
	PatTypeTest
	PatRange
)

type PatInfo struct {
	Kind    PatKind
	Variant EntityID
	Type    TypeID
	Fields  []EntityID
	Moves   bool
	// Borrow marks a pattern matched through a borrow: its bindings alias
	// the scrutinee instead of taking anything from it.
	Borrow bool
	// Mut is Borrow's mutability (a `for x in &mut xs` head, unlike match).
	Mut bool
}

type MatchInfo struct {
	Exhaustive      bool
	Irrefutable     bool
	Missing         []string
	Unreachable     []int
	AssumedExcluded []EntityID
}

// Place is the canonical path from a binding through fields, interned in
// Result.Places so narrowing and later ownership passes share identities.
type Place struct {
	Root    EntityID
	Fields  []EntityID
	Index   bool
	Deref   bool
	Stable  bool
	Mutable bool
}

func (p Place) key() string {
	var sb strings.Builder
	sb.WriteString(strconv.FormatUint(uint64(p.Root), 10))
	for _, f := range p.Fields {
		sb.WriteByte('.')
		sb.WriteString(strconv.FormatUint(uint64(f), 10))
	}
	if p.Index {
		sb.WriteString("[]")
	}
	if p.Deref {
		sb.WriteByte('*')
	}
	return sb.String()
}

func (r *Result) internPlace(p Place) PlaceID {
	k := p.key()
	if id, ok := r.placeIndex[k]; ok {
		return id
	}
	r.Places = append(r.Places, p)
	id := PlaceID(len(r.Places) - 1)
	r.placeIndex[k] = id
	return id
}

func (r *Result) place(id PlaceID) *Place { return &r.Places[id] }
