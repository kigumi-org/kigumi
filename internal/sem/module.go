package sem

import "kigumi/internal/token"

import "kigumi/internal/syntax"

type Target uint8

const (
	Hosted Target = iota
	Freestanding
)

// Package is one directory of parsed files. Module is "" for the main
// module, "std" for the stubs, any other name for a dependency.
type Package struct {
	Path   string
	Module string
	Files  []*syntax.Tree
	Std    bool
	Entry  *syntax.Tree
}

type Module struct {
	Packages []*Package
	Target   Target
	// Sources numbers files across the module so diagnostics can point into any of them.
	Sources *token.SourceStore
	// derived marks the second pass, which carries the generated codecs.
	derived bool
	// NoDefaultAllocator is the `allocator = none` profile: code
	// reachable from the entry may allocate only inside `allocator` blocks.
	NoDefaultAllocator bool
	// NoPlatform is the `--sys none` profile. Freestanding
	// implies it, but the driver sets it independently so a hosted target can
	// opt in too.
	NoPlatform bool
	// PtrBits is the target pointer width (usize, isize, raw pointers);
	// 0 means 64.
	PtrBits int
	// Arch names the target architecture (amd64, arm64, riscv64, 386, arm,
	// wasm32) for inline asm; "" means amd64.
	Arch string
	// Deny, LayerCeiling and ArtifactName mirror Artifact.availability's
	// declaration: denied import paths, the
	// layer ceiling, and the diagnostic's artifact name.
	Deny         []string
	LayerCeiling string
	ArtifactName string
}
