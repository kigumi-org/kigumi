package llgen

import (
	"fmt"
	"sort"
	"strings"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

func (e *emitter) cstr(s string) string {
	if name, ok := e.strs[s]; ok {
		return name
	}
	name := fmt.Sprintf("@.s%d", len(e.strList))
	e.strs[s] = name
	e.strList = append(e.strList, s)
	return name
}

func llEscape(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c < 0x7f && c != '"' && c != '\\' {
			sb.WriteByte(c)
		} else {
			fmt.Fprintf(&sb, "\\%02X", c)
		}
	}
	return sb.String()
}

// numKind encodes an integer or float type: bits, signedness, floatness.
func (e *emitter) numKind(t sem.TypeID) int { return mir.NumKind(e.r.Types, t) }

// Must mirror %rt_type and %rt_variant in the header (emit.go).
var rtTypeStruct = StructType{Name: "%rt_type", Fields: []Type{TPtr, TI32, TI32, TPtr, TI32, TPtr, TPtr, TI64}}
var rtVariantStruct = StructType{Name: "%rt_variant", Fields: []Type{TPtr, TPtr, TI32, TI32}}

// Bit 10 of rt_convert's dnk parameter means "target is Char" (rt_core.c).
const rtConvertCharTarget = 1 << 10

func (e *emitter) descOfType(t sem.TypeID) string {
	n := e.r.Types.Node(t)
	if n.Kind == sem.KNamed {
		return e.desc(n.Ent)
	}
	return e.desc(e.primEnt(t))
}

func (e *emitter) primEnt(t sem.TypeID) sem.EntityID {
	name := e.r.TypeString(t)
	if ent := e.r.EntityOfName(name); ent != 0 {
		return ent
	}
	return sem.EntityID(1<<20 + int(t))
}

func (e *emitter) typeCount() int { return e.ntypes }

// borrowFieldMask bits the fields declared as a borrow (`&'a T`), so
// rt_release/rt_copy skip a field the record never owned.
func (e *emitter) borrowFieldMask(info *sem.TypeDeclInfo) uint64 {
	var mask uint64
	for i, f := range info.Fields {
		if e.r.Types.Kind(e.r.Entity(f).Type) == sem.KRef {
			mask |= 1 << uint(i)
		}
	}
	return mask
}

// data emits descriptors for all named types, not just used ones, so the
// runtime can construct std records by name.
func (e *emitter) data() string {
	var sb strings.Builder
	var typeRefs []string
	for id := 1; id < len(e.r.Entities); id++ {
		ent := sem.EntityID(id)
		x := e.r.Entity(ent)
		if x.Kind != sem.EntType {
			continue
		}
		info := e.r.TypeDecl(ent)
		for _, v := range info.Variants {
			e.vdesc(v)
		}
		name := e.desc(ent)
		typeRefs = append(typeRefs, name)
		fields := "null"
		if len(info.Fields) > 0 {
			var fn []string
			for _, f := range info.Fields {
				fn = append(fn, e.cstr(e.r.Entity(f).Name))
			}
			fields = e.ptrTable(subSymbol(name, "fields"), fn)
		}
		variants := "null"
		if len(info.Variants) > 0 {
			var vn []string
			for _, v := range info.Variants {
				vn = append(vn, e.vdesc(v))
			}
			variants = e.ptrTable(subSymbol(name, "variants"), vn)
		}
		drop := "null"
		if info.Drop != 0 {
			if fn, ok := e.p.ByEnt[info.Drop]; ok {
				drop = e.adapterSym(fn)
			} else if e.p.IsStdPrimitive(info.Drop) {
				drop = e.stdAdapterSym(info.Drop)
				e.stdDrops[info.Drop] = true
			}
		}
		kind := 0
		switch {
		case info.Form == sem.FormResource:
			kind = 1
		case info.Form == sem.FormAdt:
			kind = 2
		case ent == e.r.LangItem("Range"):
			kind = 3
		}
		e.irb(&sb).GlobalStruct(name, rtTypeStruct,
			vptr(e.cstr(e.r.Packages[x.Pkg].Path+"."+x.Name)), vi32(kind), vi32(len(info.Fields)),
			vptr(fields), vi32(len(info.Variants)), vptr(variants), vptr(drop), vi64(int(e.borrowFieldMask(info))))
		for i, v := range info.Variants {
			e.irb(&sb).GlobalStruct(e.vdesc(v), rtVariantStruct, vptr(e.cstr(e.r.Entity(v).Name)), vptr(name), vi32(i), vi32(len(e.r.Variant(v).Payload)))
		}
		sb.WriteString(e.tables.String())
		e.tables.Reset()
	}
	descEnts := make([]int, 0, len(e.descs))
	for ent := range e.descs {
		descEnts = append(descEnts, int(ent))
	}
	sort.Ints(descEnts)
	for _, id := range descEnts {
		ent, name := sem.EntityID(id), e.descs[sem.EntityID(id)]
		if e.r.Entity(ent).Kind != sem.EntType || int(ent) >= len(e.r.Entities) {
			label := e.r.Entity(ent).Name
			if int(ent) >= len(e.r.Entities) {
				label = "prim"
			}
			e.irb(&sb).GlobalStruct(name, rtTypeStruct, vptr(e.cstr(label)), vi32(0), vi32(0), vptr("null"), vi32(0), vptr("null"), vptr("null"), vi64(0))
		}
	}
	drops := make([]int, 0, len(e.stdDrops))
	for ent := range e.stdDrops {
		drops = append(drops, int(ent))
	}
	sort.Ints(drops)
	for _, ent := range drops {
		sb.WriteString(e.stdAdapter(sem.EntityID(ent)))
	}
	e.ntypes = len(typeRefs)
	sb.WriteString(e.ptrTableDef("@rt_types", typeRefs))
	var pool strings.Builder
	for i, s := range e.strList {
		fmt.Fprintf(&pool, "@.s%d = private constant [%d x i8] c\"%s\\00\"\n", i, len(s)+1, llEscape(s))
	}
	return pool.String() + sb.String() + "\n"
}
