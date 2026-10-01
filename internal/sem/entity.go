package sem

import "kigumi/internal/syntax"

// EntityID indexes Result.Entities; 0 = none. The slice is append-only.
type EntityID uint32

// PackageID indexes Result.Packages; 0 is the universe pseudo-package.
type PackageID uint32

// FileID indexes Result.Files; 0 means "no file" (predeclared).
type FileID uint32

type ScopeID uint32

type OverloadSetID uint32

type PlaceID uint32

type EntityKind uint8

const (
	EntNone EntityKind = iota
	EntPackage
	EntImport
	EntType
	EntAlias
	EntInterface
	EntConstraint
	EntTypeParam
	EntFn
	EntConst
	EntVariant
	EntField
	EntLocal
	EntParam
	EntClosure
	EntContract
	EntIntrinsic
	EntTest
	EntImplicitMain
)

var entityKindNames = [...]string{
	"none", "package", "import", "type", "alias", "interface", "constraint",
	"type parameter", "function", "const", "variant", "field", "local", "parameter",
	"closure", "contract block", "intrinsic", "test", "implicit main",
}

func (k EntityKind) String() string { return entityKindNames[k] }

type EntityFlags uint16

const (
	EfMut EntityFlags = 1 << iota
	EfSelf
	EfMove
	EfVariadic
	EfStd
	EfPoison
	EfUsed
	EfReexport
	EfScript
	EfCapturedMut
	EfNamedPayload
	EfOperator
	EfEntryOnly
	// EfAddrTaken marks a local/parameter that a scalar `&mut` is
	// taken of: like EfCapturedMut, MIR gives it a cell so the borrow can
	// alias its storage instead of a disconnected copy.
	EfAddrTaken
)

type Entity struct {
	Kind   EntityKind
	Name   string
	Pkg    PackageID
	File   FileID
	Node   syntax.NodeID
	Tok    uint32
	Vis    Visibility
	Type   TypeID
	Parent EntityID
	Flags  EntityFlags
	Detail uint32
	Target EntityID
}

type VisLevel uint8

const (
	VisPrivate VisLevel = iota
	VisSuper
	VisIn
	VisModule
	VisPub
)

var visLevelNames = [...]string{"private", "pub(super)", "pub(in)", "pub(module)", "pub"}

func (v VisLevel) String() string { return visLevelNames[v] }

// Visibility is a level plus the root package of the visible range (the
// declaring package for private, its parent for super, P for pub(in P)).
type Visibility struct {
	Level VisLevel
	Scope PackageID
}

type RecvKind uint8

const (
	RecvNone RecvKind = iota
	RecvSelf
	RecvMut
	RecvMove
)

type TypeForm uint8

const (
	FormRecord TypeForm = iota
	FormResource
	FormAdt
	FormOpaque
)

type sizeState uint8

const (
	sizeUnknown sizeState = iota
	sizeComputing
	sizeFinite
	sizeInfinite
)

func (r *Result) newEntity(e Entity) EntityID {
	id := EntityID(len(r.Entities))
	r.Entities = append(r.Entities, e)
	return id
}

func (r *Result) Entity(id EntityID) *Entity { return &r.Entities[id] }

func (r *Result) poisoned(id EntityID) bool {
	return id == 0 || r.Entities[id].Flags&EfPoison != 0
}

func (r *Result) entityName(id EntityID) string {
	if id == 0 {
		return "?"
	}
	return r.Entities[id].Name
}
