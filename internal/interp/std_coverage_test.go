package interp_test

import (
	"bytes"
	"testing"

	"kigumi/internal/interp"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
	"kigumi/internal/testkit"
	"kigumi/internal/token"
)

// TestStdPrimitives pins the contract between std and the interpreter: every
// bodiless std function has a Go implementation, and no implementation is
// left behind once a function gains a Kigumi body.
func TestStdPrimitives(t *testing.T) {
	main := &sem.Package{Path: "main"}
	main.Entry = syntax.Parse(token.NewFile("main/main.kg", []byte("print \"x\"\n")))
	main.Files = []*syntax.Tree{main.Entry}
	res := sem.Check(&sem.Module{Packages: append(testkit.LoadStd(t, "../../std"), main)})
	if res.HasErrors() {
		t.Fatalf("check errors:\n%s", res.Render())
	}
	in := interp.New(res, &bytes.Buffer{}, &bytes.Buffer{}, nil)
	want := map[string]bool{}
	for id := range res.Entities {
		fn := sem.EntityID(id)
		key, ok := primitiveKey(res, fn)
		if !ok {
			continue
		}
		want[key] = true
		if !in.HasBuiltin(fn) {
			t.Errorf("%s: bodiless std function without a Go implementation", key)
		}
	}
	if len(want) < 40 {
		t.Fatalf("only %d bodiless std functions found; std not loaded as std?", len(want))
	}
	for _, key := range in.BuiltinKeys() {
		if !want[key] {
			t.Errorf("%s: Go implementation for a function that is not a bodiless std function", key)
		}
	}
	bodied := map[string]bool{}
	for id := range res.Entities {
		if key, ok := bodiedKey(res, sem.EntityID(id)); ok {
			bodied[key] = true
		}
	}
	for _, key := range in.AccelKeys() {
		if !bodied[key] {
			t.Errorf("%s: accelerator for a function that has no Kigumi body", key)
		}
	}
}

// bodiedKey names a std function that has a Kigumi body.
func bodiedKey(res *sem.Result, fn sem.EntityID) (string, bool) {
	e := res.Entity(fn)
	if e.Kind != sem.EntFn || e.Flags&sem.EfStd == 0 || e.File == 0 || res.Fn(fn).Body == 0 {
		return "", false
	}
	info := res.Fn(fn)
	key := res.Packages[e.Pkg].Path + "."
	if info.Owner != 0 {
		key += res.Entity(info.Owner).Name + "."
	}
	return key + e.Name, true
}

// primitiveKey names a bodiless std function the way the backends key it;
// `branch` is left out because `||` is lowered without calling it, and
// std/build because a build program runs only on the VM.
func primitiveKey(res *sem.Result, fn sem.EntityID) (string, bool) {
	e := res.Entity(fn)
	if e.Kind != sem.EntFn || e.Flags&sem.EfStd == 0 || e.File == 0 || e.Name == "branch" || res.Packages[e.Pkg].Path == "std/build" {
		return "", false
	}
	info := res.Fn(fn)
	if info.Body != 0 || info.Abi != "" || res.Entity(e.Parent).Kind == sem.EntInterface {
		return "", false
	}
	key := res.Packages[e.Pkg].Path + "."
	if info.Owner != 0 {
		key += res.Entity(info.Owner).Name + "."
	}
	return key + e.Name, true
}
