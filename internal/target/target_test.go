package target

import "testing"

// TestForArchKnown covers the archs added (Go GOARCH values
// kigumi didn't size before): mips/mipsle are 32 bit, the rest are 64 bit.
func TestForArchKnown(t *testing.T) {
	cases := map[string]int{
		"amd64": 64, "arm64": 64, "riscv64": 64, "386": 32, "arm": 32, "wasm32": 32,
		"mips": 32, "mipsle": 32, "mips64": 64, "mips64le": 64,
		"ppc64": 64, "ppc64le": 64, "s390x": 64, "loong64": 64,
	}
	for arch, want := range cases {
		if got := ForArch("linux", arch).PtrBits; got != want {
			t.Errorf("ForArch(linux, %s).PtrBits = %d, want %d", arch, got, want)
		}
	}
}

// TestForArchUnknownPanics: an arch outside archBits must not silently
// default to 64 bit.
func TestForArchUnknownPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("ForArch(sparc) should panic, not default to 64 bit")
		}
	}()
	ForArch("linux", "sparc")
}
