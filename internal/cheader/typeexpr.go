package cheader

import "strings"

// mapType converts one zig translate-c type expression to a Kigumi ABI-safe
// type, or ok=false when it uses a construct this importer does not
// carry across (a fixed array, a C union, `c_longdouble`, an unresolved
// identifier). known holds the Kigumi names already decided for this
// header's structs, opaque types and aliases, so a field or parameter that
// refers to one of them resolves instead of failing as "unknown".
func mapType(s string, known, opaque map[string]bool, abi ABI) (string, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "noalias ")
	// zig always spells a C pointer as a pointer to a pointee; each prefix
	// here is one such spelling, paired with the Kigumi marker it becomes.
	pointerPrefixes := []struct {
		prefix, marker string
	}{
		{"?*const ", "*const "}, {"[*c]const ", "*const "}, {"*const ", "*const "},
		{"?*", "*mut "}, {"[*c]", "*mut "}, {"*", "*mut "},
	}
	for _, p := range pointerPrefixes {
		inner, ok := strings.CutPrefix(s, p.prefix)
		if !ok {
			continue
		}
		if strings.HasPrefix(inner, "fn ") || strings.HasPrefix(inner, "fn(") {
			// `extern(C) fn(A) -> R` is already Kigumi's C function pointer
			// type; zig's `?*const fn (...) ...` names the same
			// thing, not a pointer to one, so no `*const`/`*mut` wrapper.
			return mapFnPointer(inner, known, opaque, abi)
		}
		// inner sits right behind a pointer, so an opaque type here is
		// ABI-safe (a raw pointer to it); only a bare, by-value opaque use
		// is not (mapByValueType, not this call, rejects that).
		kt, ok := mapType(inner, known, opaque, abi)
		return p.marker + kt, ok
	}
	if strings.HasPrefix(s, "fn ") || strings.HasPrefix(s, "fn(") {
		return mapFnPointer(s, known, opaque, abi)
	}
	if s == "anyopaque" {
		// Only reachable behind one of the pointer prefixes above; a bare
		// void payload has no Kigumi ABI-safe type of its own.
		return "u8", true
	}
	if t, ok := mapScalar(s, abi); ok {
		return t, true
	}
	if isIdent(s) {
		if known[s] {
			return s, true
		}
		if stripped := stripTagPrefix(s); known[stripped] {
			return stripped, true
		}
	}
	return "", false
}

// mapFnPointer converts the `fn (A, B) callconv(.c) R` tail left after a
// pointer prefix (?*const, [*c], ...) was stripped, to
// `extern(C) fn(A, B) -> R`.
func mapFnPointer(s string, known, opaque map[string]bool, abi ABI) (string, bool) {
	open := strings.IndexByte(s, '(')
	if open < 0 {
		return "", false
	}
	closeIdx := matchParen(s, open)
	if closeIdx < 0 {
		return "", false
	}
	params := splitTopCommas(s[open+1 : closeIdx])
	var kgParams []string
	variadic := false
	for _, p := range params {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "..." {
			variadic = true
			continue
		}
		// A function-pointer type's parameters are bare types, unlike a
		// function declaration's named ones, and carried by value like any
		// other C function parameter.
		kt, ok := mapByValueType(p, known, opaque, abi)
		if !ok {
			return "", false
		}
		kgParams = append(kgParams, kt)
	}
	rest := strings.TrimSpace(s[closeIdx+1:])
	rest = strings.TrimPrefix(rest, "callconv(.c)")
	rest = strings.TrimSpace(rest)
	ret := "Unit"
	if rest != "" {
		r, ok := mapByValueType(rest, known, opaque, abi)
		if !ok {
			return "", false
		}
		ret = r
	}
	if variadic {
		kgParams = append(kgParams, "...")
	}
	return "extern(C) fn(" + strings.Join(kgParams, ", ") + ") -> " + ret, true
}

// splitParam splits one `name: type` parameter zig gave a function; a
// parameter with no name (rare; zig usually invents `arg_N`) is treated as
// unnamed and rejected by the caller instead of guessing one.
func splitParam(p string) (name, typ string, ok bool) {
	i := strings.IndexByte(p, ':')
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(p[:i]), strings.TrimSpace(p[i+1:]), true
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// matchParen returns the index of the `(` at open's matching `)`, or -1.
func matchParen(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitTopCommas splits s on commas outside any nested (), [] or {}.
func splitTopCommas(s string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
