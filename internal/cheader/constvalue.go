package cheader

import "strings"

func classifyConst(name, typ, literal string, abi ABI) item {
	kt, ok := mapScalar(typ, abi)
	if !ok {
		return item{name: name, skip: "unsupported constant type `" + typ + "`"}
	}
	return item{kind: "const", name: name, text: "const " + name + ": " + kt + " = " + literal, scalar: kt, lit: literal}
}

// classifyBareConst disambiguates `pub const NAME = TARGET;`, which zig uses
// for a plain alias, a function-pointer typedef, and a macro constant alike.
func classifyBareConst(chunk string, known, opaque map[string]bool, abi ABI) item {
	m := reConstBare.FindStringSubmatch(chunk)
	name, target := m[1], strings.TrimSpace(m[2])
	switch {
	case isIdent(target):
		return classifyAlias(name, target, known, opaque, abi)
	case looksLikeType(target):
		// A function-pointer typedef (`Callback = ?*const fn (...) ...`):
		// zig's `pub const` covers both values and types, so a pointer or
		// `fn` prefix is the only signal this is the latter.
		kt, ok := mapType(target, known, opaque, abi)
		if !ok {
			return item{name: name, skip: "unsupported function pointer type"}
		}
		friendly := stripTagPrefix(name)
		known[friendly] = true
		return item{kind: "alias", name: friendly, text: "type " + friendly + " = " + kt}
	}
	typ, lit, ok := parseConstValue(target, abi)
	if !ok {
		return item{name: name, skip: "unsupported constant value"}
	}
	return item{kind: "const", name: name, text: "const " + name + ": " + typ + " = " + lit}
}

func looksLikeType(s string) bool {
	for _, p := range []string{"?*", "[*c]", "*", "fn "} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// classifyAlias handles `pub const NAME = TARGET;` where TARGET is a bare
// identifier: either a scalar keyword (an imported enum's underlying type
// becomes a named alias to it) or another name this pass already emitted.
// The classic C `typedef struct X { ... } X;` shape reaches here as its own
// redundant alias chunk (X = struct_X) once the struct itself is emitted
// under the stripped name X; that case is silently coalesced away.
func classifyAlias(name, target string, known, opaque map[string]bool, abi ABI) item {
	if kt, ok := mapScalar(target, abi); ok {
		friendly := stripTagPrefix(name)
		known[friendly] = true
		return item{kind: "alias", name: friendly, text: "type " + friendly + " = " + kt, scalar: kt}
	}
	resolved := target
	if !known[resolved] {
		if s := stripTagPrefix(target); known[s] {
			resolved = s
		} else {
			return item{name: name, skip: "alias to unresolved `" + target + "`"}
		}
	}
	friendly := stripTagPrefix(name)
	if friendly == resolved {
		return item{kind: "noop"}
	}
	known[friendly] = true
	// An alias to an opaque name is itself pointer-only: `typedef struct
	// Bitfield Bitfield2;` must reject a by-value `Bitfield2` just like
	// `Bitfield`.
	if opaque[resolved] {
		opaque[friendly] = true
	}
	return item{kind: "alias", name: friendly, text: "type " + friendly + " = " + resolved}
}

// parseConstValue reads the value zig gave a `#define`d constant: a signed
// or unsigned integer literal (`@as(TYPE, N)` or, once it no longer fits a
// plain int, `__helpers.promoteIntLiteral(TYPE, N, .decimal)`), a float
// literal (`@as(f32|f64, N)`), or a bare string literal.
func parseConstValue(target string, abi ABI) (typ, literal string, ok bool) {
	neg := ""
	if s, hasNeg := strings.CutPrefix(target, "-"); hasNeg {
		neg, target = "-", s
	}
	for _, wrapper := range []string{"@as(", "__helpers.promoteIntLiteral("} {
		if inner, hasWrap := strings.CutPrefix(target, wrapper); hasWrap && strings.HasSuffix(inner, ")") {
			parts := splitTopCommas(inner[:len(inner)-1])
			if len(parts) < 2 {
				return "", "", false
			}
			kt, ok := mapScalar(strings.TrimSpace(parts[0]), abi)
			if !ok {
				return "", "", false
			}
			return kt, neg + strings.TrimSpace(parts[1]), true
		}
	}
	if neg == "" && len(target) >= 2 && target[0] == '"' && target[len(target)-1] == '"' {
		return "String", target, true
	}
	return "", "", false
}
