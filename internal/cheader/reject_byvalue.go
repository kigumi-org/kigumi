package cheader

import "strings"

// bareOpaqueByValue reports whether s (a raw zig type expression, not yet
// run through mapType) names a type opaque registered as pointer-only —
// zig demoted it (usually for a bitfield) and Kigumi has no way to lay it
// out, so only a pointer to it is ABI-safe (not a by-value opaque payload).
func bareOpaqueByValue(s string, opaque map[string]bool) bool {
	s = strings.TrimPrefix(strings.TrimSpace(s), "noalias ")
	if opaque[s] {
		return true
	}
	return opaque[stripTagPrefix(s)]
}

// mapByValueType is mapType for a position the C ABI carries by value (a
// function parameter or result, a record field): unlike a pointer's
// pointee, a bare opaque type is rejected here instead of resolving, so the
// importer skips the one declaration instead of emitting Kigumi source that
// sem's own ABI checker (E924) would then refuse the moment the generated
// package is merely imported.
func mapByValueType(s string, known, opaque map[string]bool, abi ABI) (string, bool) {
	if bareOpaqueByValue(s, opaque) {
		return "", false
	}
	return mapType(s, known, opaque, abi)
}
