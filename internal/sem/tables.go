package sem

import (
	"kigumi/internal/diag"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// FileInfo is the side table of one syntax.Tree; dense slices are indexed by NodeID.
type FileInfo struct {
	Tree   *syntax.Tree
	Pkg    PackageID
	Types  []TypeID
	Uses   []EntityID
	Defs   []EntityID
	Scopes []ScopeID

	Coerce   map[syntax.NodeID]Coercion
	Calls    map[syntax.NodeID]CallInfo
	Pats     map[syntax.NodeID]PatInfo
	Matches  map[syntax.NodeID]MatchInfo
	Places   map[syntax.NodeID]PlaceID
	Narrow   map[syntax.NodeID][]NarrowFact
	Literals map[syntax.NodeID]constValue
	Asm      map[syntax.NodeID]*AsmInfo
	Diags    []diag.Diagnostic
}

func newFileInfo(t *syntax.Tree, pkg PackageID) FileInfo {
	n := len(t.Nodes)
	return FileInfo{
		Tree: t, Pkg: pkg,
		Types: make([]TypeID, n), Uses: make([]EntityID, n), Defs: make([]EntityID, n), Scopes: make([]ScopeID, n),
		Coerce: map[syntax.NodeID]Coercion{}, Calls: map[syntax.NodeID]CallInfo{}, Pats: map[syntax.NodeID]PatInfo{},
		Matches: map[syntax.NodeID]MatchInfo{}, Places: map[syntax.NodeID]PlaceID{},
		Narrow: map[syntax.NodeID][]NarrowFact{}, Literals: map[syntax.NodeID]constValue{},
		Asm: map[syntax.NodeID]*AsmInfo{},
	}
}

type CoKind uint8

const (
	CoLiteral CoKind = iota + 1
	CoSome
	CoOk
	CoExistential
	CoFnItem
	CoClosureErase
	CoNever
	// CoBorrow lends a Copy value to a `&T` parameter through a temporary.
	CoBorrow
	// CoCFnPtr turns an export(C) function item into a C function pointer.
	CoCFnPtr
)

type CoStep struct {
	Kind CoKind
	To   TypeID
}

// Coercion lists the steps applied to a synthesized value, innermost first.
type Coercion struct {
	From  TypeID
	Steps []CoStep
}

type FactKind uint8

const (
	FactIs FactKind = iota
	FactIsNot
	// FactPureField remembers a `pure?` field's bound purity at a place.
	FactPureField
)

type NarrowFact struct {
	Place   PlaceID
	Kind    FactKind
	Variant EntityID
	Mods    Effects
	Target  EntityID
	Param   EntityID
}

type EdgeKind uint8

const (
	EdgeCall EdgeKind = iota + 1
	EdgeCallback
	EdgeWitness
	EdgeIO
	EdgeMutateCaller
	EdgeUnsafe
	EdgeAlloc
	EdgeDrop
	EdgeContract
	EdgePanic
	EdgeParamCall
	// EdgeWitnessCall is a `pure?` requirement deferred to the caller's
	// resolved witness.
	EdgeWitnessCall
)

type EffectEdge struct {
	Kind   EdgeKind
	Target EntityID
	Param  EntityID
	Mods   Effects
	Type   TypeID
	From   TypeID
	File   FileID
	Node   syntax.NodeID
	Why    uint8
	// Scoped edges sit inside an `allocator` block, which supplies the
	// current allocator for their dynamic extent.
	Scoped bool
}

type BodyRef struct {
	Fn   EntityID
	File FileID
	Node syntax.NodeID
}

type Instance struct {
	Args []TypeID
	File FileID
	Node syntax.NodeID
	In   EntityID
}

func (f *FileInfo) span(n syntax.NodeID) token.Span { return f.Tree.Span(n) }
