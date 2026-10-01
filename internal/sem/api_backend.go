package sem

import (
	"math/big"

	"kigumi/internal/syntax"
)

// Accessors for the back ends (mir, interp, llgen): tables stay unexported
// so the checker owns them, and readers go through here.

// FnCVariadic is the fn type flag of a C `...` signature; FnCAbi marks a C
// function pointer type.
const (
	FnCVariadic = fnCVariadic
	FnCAbi      = fnCAbi
)

type LitKind uint8

const (
	LitInt LitKind = iota + 1
	LitFloat
	LitString
	LitBool
	LitChar
	LitBytes
)

type Literal struct {
	Kind  LitKind
	Int   *big.Int
	Float *big.Float
	Str   string
	Bool  bool
	Char  rune
}

func (v constValue) literal() (Literal, bool) {
	out := Literal{Int: v.Int, Float: v.Float, Str: v.Str, Bool: v.Bool, Char: v.Char}
	switch v.Kind {
	case constInt:
		out.Kind = LitInt
	case constFloat:
		out.Kind = LitFloat
	case constString:
		out.Kind = LitString
	case constBytes:
		out.Kind = LitBytes
	case constBool:
		out.Kind = LitBool
	case constChar:
		out.Kind = LitChar
	default:
		return Literal{}, false
	}
	return out, true
}

// LiteralOf returns the folded value of a literal node.
func (r *Result) LiteralOf(t *syntax.Tree, n syntax.NodeID) (Literal, bool) {
	f := r.File(t)
	if f == nil {
		return Literal{}, false
	}
	v, ok := f.Literals[n]
	if !ok {
		return Literal{}, false
	}
	return v.literal()
}

// ConstValueOf returns the evaluated value of a const entity.
func (r *Result) ConstValueOf(id EntityID) (Literal, bool) {
	if r.Entities[id].Kind != EntConst {
		return Literal{}, false
	}
	return r.constInfo(id).Value.literal()
}

func (r *Result) TypeDecl(id EntityID) *TypeDeclInfo       { return r.typeDecl(id) }
func (r *Result) AsmOf(f FileID, n syntax.NodeID) *AsmInfo { return r.Files[f].Asm[n] }
func (r *Result) Variant(id EntityID) *VariantInfo         { return r.variant(id) }
func (r *Result) Closure(id EntityID) *ClosureInfo         { return r.closure(id) }
func (r *Result) Field(id EntityID) *FieldInfo             { return r.field(id) }
func (r *Result) Local(id EntityID) *LocalInfo             { return r.local(id) }
func (r *Result) TypeParam(id EntityID) *TypeParamInfo     { return r.typeParam(id) }
func (r *Result) Iface(id EntityID) *InterfaceInfo         { return r.iface(id) }

// Witnesses lists the methods of t that implement iface's requirements,
// in requirement order, as seen from package from; nil when t does not
// conform.
func (r *Result) Witnesses(t, iface TypeID, from PackageID) []EntityID {
	res := r.conforms(t, iface, from)
	if !res.ok {
		return nil
	}
	return res.witnesses
}
func (r *Result) Tree(f FileID) *syntax.Tree            { return r.tree(f) }
func (r *Result) PackageOf(f FileID) PackageID          { return r.packageOf(f) }
func (r *Result) ImplicitMains() map[PackageID]EntityID { return r.implicitMain }
func (r *Result) LangItem(name string) EntityID         { return r.langItems[name] }
func (r *Result) Requirement(iface EntityID, name string) EntityID {
	return r.requirement(iface, name)
}
func (r *Result) MemberSet(t TypeID, name string) OverloadSetID  { return r.memberSet(t, name) }
func (r *Result) FindField(owner EntityID, name string) EntityID { return r.findField(owner, name) }
func (r *Result) IsCopy(t TypeID) bool                           { return r.isCopy(t) }
func (r *Result) IsSend(t TypeID) bool                           { return r.isSend(t) }
func (r *Result) IsSync(t TypeID) bool                           { return r.isSync(t) }
func (r *Result) Summary(id EntityID) EffectSummary              { return r.summary(id) }

// MainFn names the process entry: the implicit main of an entry file, or an
// explicit top-level `fn main` when the module has no script file.
func (r *Result) MainFn() EntityID {
	var implicit, explicit EntityID
	for id := 1; id < len(r.Entities); id++ {
		e := &r.Entities[id]
		if e.File == 0 || e.Flags&EfPoison != 0 {
			continue
		}
		if p := &r.Packages[e.Pkg]; p.Std || p.Module != "" {
			continue
		}
		switch {
		case e.Kind == EntImplicitMain && implicit == 0:
			implicit = EntityID(id)
		case e.Kind == EntFn && e.Name == "main" && explicit == 0 && r.Fn(EntityID(id)).Owner == 0:
			explicit = EntityID(id)
		}
	}
	if implicit != 0 {
		return implicit
	}
	return explicit
}

// PackageMember looks a top-level name up in a package's own scope.
func (r *Result) PackageMember(pkg PackageID, name string) EntityID {
	b, ok := r.Scopes[r.Packages[pkg].Scope].Names[name]
	if !ok {
		return 0
	}
	if b.Ent != 0 {
		return r.follow(b.Ent)
	}
	return r.Overloads[b.Set].Members[0]
}

// PackageByPath finds a package by import path.
func (r *Result) PackageByPath(path string) PackageID {
	for i := range r.Packages {
		if r.Packages[i].Path == path {
			return PackageID(i)
		}
	}
	return 0
}

func (tt *TypeTable) OptionEnt() EntityID { return tt.optionEnt }
func (tt *TypeTable) ResultEnt() EntityID { return tt.resultEnt }
func (tt *TypeTable) ArrayEnt() EntityID  { return tt.arrayEnt }
func (tt *TypeTable) FutureEnt() EntityID { return tt.futureEnt }
func (tt *TypeTable) ErrorEnt() EntityID  { return tt.errorEnt }

// TupleEntity is tupleEntity, exported for the backends that construct a
// tuple value directly (a Map's `for` element).
func (tt *TypeTable) TupleEntity(n int) (EntityID, bool) { return tt.tupleEntity(n) }

// MapEntity is mapEntity, exported for the backends that recognize a Map
// container at runtime.
func (r *Result) MapEntity() EntityID { return r.mapEntity() }

const (
	CapCopy   = capCopy
	CapRetain = capRetain
	CapMove   = capMove
	CapCell   = capCell
	CapBorrow = capBorrow
)
