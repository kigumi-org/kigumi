package sem

import (
	"strconv"
	"strings"
)

// TypeID indexes TypeTable.nodes; 0 means "not computed".
type TypeID uint32

// Fixed ids, interned by newTypeTable in this order.
const (
	NoType TypeID = iota
	TyPoison
	TyNever
	TyUnit
	TyBool
	TyChar
	TyString
	TyBytes
	TyI8
	TyI16
	TyI32
	TyI64
	TyI128
	TyU8
	TyU16
	TyU32
	TyU64
	TyU128
	TyUsize
	TyIsize
	TyF32
	TyF64
	TyUntypedInt
	TyUntypedFloat
	firstDynamicType
)

type TypeKind uint8

const (
	KPoison TypeKind = iota
	KPrim
	KUntyped
	KNamed
	KIface
	KParam
	KVar
	KFn
	KClosure
	KRef
	KPtr
	// KConst is a const generic argument: Var indexes constVals, Elem is the
	// value's declared type.
	KConst
)

// Effects shares bit positions with syntax.Mod* so Effects(mods) converts.
type Effects uint8

const (
	EffPure Effects = 1 << iota
	EffNoalloc
	EffAsync
	EffUnsafe
)

const (
	flagMut    uint16 = 1 << 8
	fnVariadic uint16 = 1 << 9
	// fnCVariadic marks a C `...` tail: any number of extra scalar arguments.
	fnCVariadic uint16 = 1 << 10
	// fnCAbi marks a C function pointer type `extern(C) fn(A) -> R`, also the
	// type of an extern(C) declaration used as a value.
	fnCAbi uint16 = 1 << 11
	// fnEffectPoly marks a `pure? fn` type; EffPure is left unset at
	// declaration and resolved per use site instead.
	fnEffectPoly uint16 = 1 << 12
)

type typeNode struct {
	Kind  TypeKind
	Flags uint16
	Ent   EntityID
	Elem  TypeID
	Var   uint32
	Args  []TypeID
}

type typeProps struct {
	copy, eq, hash, ord, send, sync int8
}

// TypeTable hash-conses typeNodes so structurally equal types share one id
// and identity is ==; aliases never appear here.
type TypeTable struct {
	nodes []typeNode
	index map[string]TypeID
	props []typeProps
	// ptrBits is the target pointer width, the width of usize and isize.
	ptrBits int

	optionEnt, resultEnt, arrayEnt, errorEnt, futureEnt EntityID
	// constVals holds the distinct const generic argument values in
	// first-seen order; constIndex dedups them to a stable index.
	constVals  []int64
	constIndex map[int64]uint32
	// tupleEnt holds Tuple2..Tuple8 at indices 2..8; 0 and 1 are unused (a
	// 1-tuple is a parenthesized expression, not a type of its own).
	tupleEnt [9]EntityID
}

// tupleEntity returns the predeclared record for an n-ary tuple (2..8), or
// false outside that range.
func (tt *TypeTable) tupleEntity(n int) (EntityID, bool) {
	if n < 2 || n > 8 {
		return 0, false
	}
	return tt.tupleEnt[n], true
}

func (tt *TypeTable) isTupleEnt(ent EntityID) bool {
	for _, e := range tt.tupleEnt {
		if e != 0 && e == ent {
			return true
		}
	}
	return false
}

func newTypeTable(ptrBits int) *TypeTable {
	tt := &TypeTable{index: map[string]TypeID{}, ptrBits: ptrBits, constIndex: map[int64]uint32{}}
	tt.nodes = append(tt.nodes, typeNode{})
	tt.props = append(tt.props, typeProps{})
	tt.Intern(typeNode{Kind: KPoison})
	for i := TyNever; i <= TyF64; i++ {
		tt.Intern(typeNode{Kind: KPrim, Var: uint32(i)})
	}
	tt.Intern(typeNode{Kind: KUntyped, Var: uint32(TyUntypedInt)})
	tt.Intern(typeNode{Kind: KUntyped, Var: uint32(TyUntypedFloat)})
	return tt
}

func encode(n typeNode) string {
	var sb strings.Builder
	sb.WriteByte(byte('A' + n.Kind))
	sb.WriteString(strconv.FormatUint(uint64(n.Flags), 10))
	sb.WriteByte(':')
	sb.WriteString(strconv.FormatUint(uint64(n.Ent), 10))
	sb.WriteByte(':')
	sb.WriteString(strconv.FormatUint(uint64(n.Elem), 10))
	sb.WriteByte(':')
	sb.WriteString(strconv.FormatUint(uint64(n.Var), 10))
	for _, a := range n.Args {
		sb.WriteByte(',')
		sb.WriteString(strconv.FormatUint(uint64(a), 10))
	}
	return sb.String()
}

func (tt *TypeTable) Intern(n typeNode) TypeID {
	key := encode(n)
	if id, ok := tt.index[key]; ok {
		return id
	}
	if n.Args != nil {
		n.Args = append([]TypeID(nil), n.Args...)
	}
	id := TypeID(len(tt.nodes))
	tt.nodes = append(tt.nodes, n)
	tt.props = append(tt.props, typeProps{})
	tt.index[key] = id
	return id
}

func (tt *TypeTable) Node(t TypeID) typeNode { return tt.nodes[t] }

func (tt *TypeTable) Kind(t TypeID) TypeKind { return tt.nodes[t].Kind }

func (tt *TypeTable) Len() int { return len(tt.nodes) }
