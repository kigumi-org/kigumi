package sem_test

import (
	"testing"

	"kigumi/internal/sem"
)

func TestTypeIntern(t *testing.T) {
	res := sem.Check(&sem.Module{})
	tt := res.Types
	a := tt.Fn([]sem.TypeID{sem.TyI64}, sem.TyI64, 0, false)
	b := tt.Fn([]sem.TypeID{sem.TyI64}, sem.TyI64, 0, false)
	if a != b {
		t.Errorf("Fn([Int], Int) interned twice: %d and %d", a, b)
	}
	if tt.Fn([]sem.TypeID{sem.TyI64}, sem.TyI64, sem.EffPure, false) == a {
		t.Error("pure fn and fn share an id")
	}
	opt := tt.Option(sem.TyI64)
	if opt == tt.Option(opt) {
		t.Error("Option(Int) == Option(Int?)")
	}
	if opt != tt.Option(sem.TyI64) {
		t.Error("Option(Int) interned twice")
	}
	if elem, ok := tt.IsOption(opt); !ok || elem != sem.TyI64 {
		t.Errorf("IsOption(Int?) = %d %v", elem, ok)
	}
	if got := res.TypeString(tt.Option(opt)); got != "Int??" {
		t.Errorf("TypeString(Int??) = %q", got)
	}
	if got := res.TypeString(tt.Result(sem.TyI64, tt.Iface(res.EntityOfName("Error"), nil))); got != "Int!" {
		t.Errorf("TypeString(Int!) = %q", got)
	}
	if got := res.TypeString(tt.Fn([]sem.TypeID{sem.TyString, tt.Option(sem.TyU8)}, sem.TyUnit, sem.EffPure|sem.EffNoalloc, false)); got != "pure noalloc fn(String, u8?) -> Unit" {
		t.Errorf("TypeString(fn) = %q", got)
	}
	if got := res.TypeString(tt.Ref(sem.TyString, true)); got != "&mut String" {
		t.Errorf("TypeString(&mut String) = %q", got)
	}
}

func TestSubst(t *testing.T) {
	res := sem.Check(&sem.Module{})
	tt := res.Types
	arr := res.EntityOfName("Array")
	tp := res.TypeDecls[res.Entity(arr).Detail].Params[0]
	p := tt.Param(tp)
	generic := tt.Named(arr, []sem.TypeID{tt.Option(p)})
	inst := tt.Subst(generic, map[sem.EntityID]sem.TypeID{tp: sem.TyBool})
	if want := tt.Named(arr, []sem.TypeID{tt.Option(sem.TyBool)}); inst != want {
		t.Errorf("Subst = %s, want %s", res.TypeString(inst), res.TypeString(want))
	}
	if tt.Subst(sem.TyI64, map[sem.EntityID]sem.TypeID{tp: sem.TyBool}) != sem.TyI64 {
		t.Error("Subst changed a primitive")
	}
	if !tt.Contains(generic, p) || tt.Contains(inst, p) {
		t.Error("Contains is wrong")
	}
	if ps := tt.Params(generic); len(ps) != 1 || ps[0] != tp {
		t.Errorf("Params = %v", ps)
	}
	if got := res.TypeString(generic); got != "Array[T?]" {
		t.Errorf("TypeString(generic) = %q", got)
	}
}
