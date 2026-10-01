package cheader

import "strings"

// ABI is the handful of C ABI facts that vary by target and that this
// importer's fixed scalar table depends on: everything else in a C ABI
// (calling convention, struct layout, alignment) is already handled by
// zig translate-c itself.
type ABI struct {
	// LLP64 is true on Windows, where c_long/c_ulong are 32-bit unlike
	// every LP64 target this importer otherwise sees.
	LLP64 bool
	// CharUnsigned is true on arm/arm64, where plain `char` defaults to
	// unsigned unlike x86/x86_64.
	CharUnsigned bool
}

// ABIFor derives ABI facts from the same os/arch names internal/driver.Target
// uses (amd64, arm64, riscv64, 386, arm; linux, darwin, windows, ...).
func ABIFor(os, arch string) ABI {
	return ABI{
		LLP64:        os == "windows",
		CharUnsigned: arch == "arm64" || arch == "arm",
	}
}

// mapScalar maps one zig translate-c scalar type keyword to the Kigumi
// ABI-safe type it stands for on this target; ok is false for a
// keyword this importer does not carry across (c_longdouble, and anything
// not in this fixed table).
func mapScalar(word string, abi ABI) (string, bool) {
	switch word {
	case "c_int":
		return "i32", true
	case "c_uint":
		return "u32", true
	case "c_short":
		return "i16", true
	case "c_ushort":
		return "u16", true
	case "c_long":
		if abi.LLP64 {
			return "i32", true
		}
		return "i64", true
	case "c_ulong":
		if abi.LLP64 {
			return "u32", true
		}
		return "u64", true
	case "c_longlong":
		return "i64", true
	case "c_ulonglong":
		return "u64", true
	case "c_char":
		if abi.CharUnsigned {
			return "u8", true
		}
		return "i8", true
	case "c_schar":
		return "i8", true
	case "c_uchar":
		return "u8", true
	// zig's own fixed-width integer names (u8, i32, ...) already spell the
	// same widths Kigumi uses, and translate-c emits them directly for a
	// stdint.h type and for plain `char` (u8); pass them through unchanged.
	case "u8", "i8", "u16", "i16", "u32", "i32", "u64", "i64":
		return word, true
	case "usize":
		return "usize", true
	case "isize", "c_ptrdiff_t":
		return "isize", true
	case "f32":
		return "f32", true
	case "f64":
		return "f64", true
	case "bool":
		// Bool is not ABI-safe; a byte is the honest carrier for
		// C's `_Bool` (0/1), chosen here rather than inventing ABI status
		// for the language's Bool.
		return "u8", true
	case "void":
		return "Unit", true
	}
	return "", false
}

// stripTagPrefix drops the struct_/union_/enum_ tag prefix zig gives an
// untypedef'd C tag, so `struct Point` and `typedef struct Point Point`
// import under the same Kigumi name.
func stripTagPrefix(name string) string {
	for _, p := range []string{"struct_", "union_", "enum_"} {
		if s, ok := strings.CutPrefix(name, p); ok {
			return s
		}
	}
	return name
}
