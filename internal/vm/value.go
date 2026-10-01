// Package vm executes MIR directly with the C runtime's value
// model: reference-counted boxed values, the ownership plan of
// mir.Plan deciding copies and releases, and drop hooks running when the
// last reference goes.
package vm

import (
	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

type kind uint8

const (
	kFree kind = iota
	kInt
	kFloat
	kBool
	kStr
	kBytes
	kChar
	kUnit
	kRecord
	kVariant
	kArray
	kClosure
	kBox
	kCell
	kOpaque
)

// obj is one runtime value; rc < 0 marks a static object that is never
// freed. Fields double as record fields, variant payload and array items.
type obj struct {
	k      kind
	rc     int32
	nk     int
	i      int64
	f      float64
	b      bool
	s      []byte
	c      rune
	ent    sem.EntityID
	fields []*obj
	fn     *mir.Func
	stdKey string
	bind   bool
	lent   int
	env    *obj
	async  bool
	dyn    sem.TypeID
	inner  *obj
	op     string
	data   any
	chg    int64
}

var (
	unitObj  = &obj{k: kUnit, rc: -1}
	trueObj  = &obj{k: kBool, rc: -1, b: true}
	falseObj = &obj{k: kBool, rc: -1}
)

func newObj(k kind) *obj { return &obj{k: k, rc: 1} }

func mkInt(i int64, nk int) *obj {
	if nk == 0 {
		nk = 64 | 256
	}
	v := newObj(kInt)
	v.i, v.nk = i, nk
	return v
}

func mkFloat(f float64, nk int) *obj {
	v := newObj(kFloat)
	if nk&255 == 32 {
		f = float64(float32(f))
	}
	v.f, v.nk = f, nk
	return v
}

func mkBool(b bool) *obj {
	if b {
		return trueObj
	}
	return falseObj
}

func mkStr(s []byte) *obj   { v := newObj(kStr); v.s = s; return v }
func mkBytes(s []byte) *obj { v := newObj(kBytes); v.s = s; return v }
func mkChar(c rune) *obj    { v := newObj(kChar); v.c = c; return v }

func mkArray(items []*obj) *obj {
	v := newObj(kArray)
	v.fields = append([]*obj{}, items...)
	return v
}

func mkRecord(ent sem.EntityID, fields []*obj) *obj {
	v := newObj(kRecord)
	v.ent, v.fields = ent, append([]*obj{}, fields...)
	return v
}

func mkVariant(ent sem.EntityID, payload []*obj) *obj {
	v := newObj(kVariant)
	v.ent, v.fields = ent, append([]*obj{}, payload...)
	return v
}

func mkCell(x *obj) *obj { v := newObj(kCell); v.inner = x; return v }

func mkOpaque(op string, data any) *obj { v := newObj(kOpaque); v.op, v.data = op, data; return v }

// deref follows cells, as every runtime entry point does.
func deref(v *obj) *obj {
	for v != nil && v.k == kCell {
		v = v.inner
	}
	return v
}

func retain(v *obj) *obj {
	if v != nil && v.rc > 0 {
		v.rc++
	}
	return v
}
