package vm_test

import (
	"testing"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
	"kigumi/internal/vm"
)

// The VM leaves foreign memory, the dynamic loader and the HTTP client to
// `kigumi build`; Supports sends such programs to the interpreter. The
// socket layer (Net.connect/listen and Conn/Listener) is accelerated
// instead (net_socket.go), so os.Host.net is not a gap here.
var vmGaps = map[string]bool{
	"std/ffi.CString.new": true, "std/ffi.CString.ptr": true, "std/ffi.CString.drop": true, "std/ffi.offset": true, "std/ffi.string": true, "std/ffi.stringChecked": true, "std/ffi.stringLossy": true,
	"std/ffi.readU8": true, "std/ffi.readI32": true, "std/ffi.readI64": true, "std/ffi.readF64": true, "std/ffi.writeU8": true,
	"std/ffi.writeI32": true, "std/ffi.writeI64": true, "std/ffi.writeF64": true, "std/ffi.bytesFrom": true, "std/ffi.bytesPtr": true,
	"std/ffi.readPtr": true, "std/ffi.writePtr": true,
	"std/net.Net.http": true, "std/os.Host.dl": true, "std/dl.Symbol.call": true,
}

// TestStdPrimitives pins the contract between std and the VM: every
// bodiless std function has an implementation, except the listed gaps.
func TestStdPrimitives(t *testing.T) {
	res := checkSource(t, "print \"x\"\n")
	if res.HasErrors() {
		t.Fatalf("check errors:\n%s", res.Render())
	}
	prog := mir.Build(res)
	n := 0
	for id := range res.Entities {
		fn := sem.EntityID(id)
		e := res.Entity(fn)
		if e.Kind != sem.EntFn || e.Flags&sem.EfStd == 0 || e.File == 0 || e.Name == "branch" {
			continue
		}
		info := res.Fn(fn)
		if info.Body != 0 || info.Abi != "" || res.Entity(e.Parent).Kind == sem.EntInterface {
			continue
		}
		key := res.Packages[e.Pkg].Path + "."
		if info.Owner != 0 {
			key += res.Entity(info.Owner).Name + "."
		}
		key += e.Name
		n++
		if vm.Implements(prog, fn) == vmGaps[key] {
			t.Errorf("%s: implemented=%v, listed as a gap=%v", key, vm.Implements(prog, fn), vmGaps[key])
		}
	}
	if n < 40 {
		t.Fatalf("only %d bodiless std functions found", n)
	}
}
