package llgen

import "kigumi/internal/mir"

// Emit renders the whole program as one LLVM module.
func Emit(p *mir.Program) string { return EmitWith(p, EmitOptions{}) }

// Output is a compiled program: an LLVM module and, when needed, a C shim
// (emit_cshim.go).
type Output struct {
	IR    string
	CShim string
}

// EmitOptions selects the entry point: a hosted `main`, or `kigumi_main`
// for a freestanding target.
type EmitOptions struct {
	Freestanding bool
	// SymbolPrefix is what the target prepends to C symbol names ("_" on
	// Mach-O); module-level asm has to spell it out explicitly.
	SymbolPrefix string
}

func EmitWith(p *mir.Program, opts EmitOptions) string { return EmitProgram(p, opts).IR }
