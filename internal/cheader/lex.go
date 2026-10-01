package cheader

import "strings"

// splitTopLevel cuts zig translate-c output into one string per top-level
// statement (a `pub const`/`pub extern fn`/`pub fn`/`pub extern var`, or the
// leading boilerplate before the first one). A `//` comment (zig's own
// translation warnings) is dropped entirely rather than folded into a
// statement; Generate scans the untouched output separately for the one
// warning it cares about (a struct demoted to opaque).
func splitTopLevel(src string) []string {
	var out []string
	depth := 0
	start := 0
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '"' || c == '\'':
			i = skipLiteral(src, i)
			continue
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			if depth == 0 {
				start = i
			}
			continue
		case c == '{' || c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			depth--
		case c == '}':
			depth--
			if depth == 0 {
				out = append(out, strings.TrimSpace(src[start:i+1]))
				i++
				for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
					i++
				}
				if i < len(src) && src[i] == ';' {
					i++
				}
				start = i
				continue
			}
		case c == ';' && depth == 0:
			out = append(out, strings.TrimSpace(src[start:i+1]))
			start = i + 1
		}
		i++
	}
	return out
}

// skipLiteral returns the index just past the string or char literal
// starting at i, so its content never affects brace depth.
func skipLiteral(src string, i int) int {
	quote := src[i]
	i++
	for i < len(src) {
		if src[i] == '\\' {
			i += 2
			continue
		}
		if src[i] == quote {
			return i + 1
		}
		i++
	}
	return i
}

// declHead reports the top-level kind ("fn", "var" or "const") and name of
// one chunk, or ok=false for boilerplate this importer never emits
// regardless of origin (`const __root = @This();` and friends).
func declHead(chunk string) (kind, name string, ok bool) {
	rest, ok := strings.CutPrefix(chunk, "pub ")
	if !ok {
		return "", "", false
	}
	rest = strings.TrimPrefix(rest, "extern ")
	rest = strings.TrimPrefix(rest, "inline ")
	for _, k := range []string{"fn", "var", "const"} {
		r, ok := strings.CutPrefix(rest, k+" ")
		if !ok {
			continue
		}
		name, _, _ = strings.Cut(strings.TrimSpace(r), "(")
		name, _, _ = strings.Cut(name, ":")
		name, _, _ = strings.Cut(name, "=")
		return k, strings.TrimSpace(name), true
	}
	return "", "", false
}
