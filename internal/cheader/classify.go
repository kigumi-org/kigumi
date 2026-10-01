package cheader

import (
	"fmt"
	"regexp"
	"strings"
)

// item is one Kigumi declaration this importer decided to emit, or a
// skipped one (text == "").
type item struct {
	kind string // "opaque", "record", "alias", "fn", "const"
	name string
	text string
	skip string
	// scalar and lit are set only for a "const" from classifyConst and for
	// an "alias" from a scalar keyword; Generate uses them to widen a run
	// of enum members when they disagree with their enum's type.
	scalar string
	lit    string
}

var reConstTyped = regexp.MustCompile(`^pub const (\w+): (\w+) = (.+);$`)
var reConstBare = regexp.MustCompile(`(?s)^pub const (\w+) = (.+);$`)
var reExternVar = regexp.MustCompile(`^pub extern var (\w+):`)
var reExternFn = regexp.MustCompile(`(?s)^pub extern fn (\w+)\((.*)$`)
var reBodiedFn = regexp.MustCompile(`^pub (?:inline )?fn (\w+)\(`)

// classify turns one header-specific chunk into an item, consulting and
// extending known (the Kigumi names already available for type references)
// as struct, opaque and alias declarations are found. Chunks must be fed in
// their original header order: a C declaration can only reference an
// earlier one, so one forward pass is enough to resolve every reference.
func classify(chunk string, known, opaque map[string]bool, abi ABI) item {
	switch {
	case strings.Contains(chunk, "@compileError("):
		return item{name: declName(chunk), skip: "unsupported C construct (zig could not translate it)"}
	case reExternVar.MatchString(chunk):
		name := reExternVar.FindStringSubmatch(chunk)[1]
		return item{name: name, skip: "extern data symbol (Kigumi has no `extern(C)` variable declaration yet)"}
	case reBodiedFn.MatchString(chunk):
		name := reBodiedFn.FindStringSubmatch(chunk)[1]
		return item{name: name, skip: "inline function or function-like macro (not a plain declaration)"}
	case reExternFn.MatchString(chunk):
		return classifyFn(chunk, known, opaque, abi)
	case strings.Contains(chunk, "= extern struct {") || strings.Contains(chunk, "= extern union {"):
		return classifyRecord(chunk, known, opaque, abi)
	case strings.Contains(chunk, "= opaque {"):
		return classifyOpaque(chunk, known, opaque)
	case reConstTyped.MatchString(chunk):
		m := reConstTyped.FindStringSubmatch(chunk)
		return classifyConst(m[1], m[2], m[3], abi)
	case reConstBare.MatchString(chunk):
		return classifyBareConst(chunk, known, opaque, abi)
	}
	return item{name: declName(chunk), skip: "unrecognized declaration shape"}
}

// declName is the best-effort name of a chunk this importer could not even
// pattern-match, so the skip list still names something instead of "?".
func declName(chunk string) string {
	if _, name, ok := declHead(chunk); ok {
		return name
	}
	return "?"
}

func classifyFn(chunk string, known, opaque map[string]bool, abi ABI) item {
	m := reExternFn.FindStringSubmatch(chunk)
	name, rest := m[1], m[2]
	closeIdx := matchParen("("+rest, 0)
	if closeIdx < 0 {
		return item{name: name, skip: "malformed function declaration"}
	}
	paramsText, tail := rest[:closeIdx-1], strings.TrimSuffix(strings.TrimSpace(rest[closeIdx:]), ";")
	var params []string
	variadic := false
	for i, p := range splitTopCommas(paramsText) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "..." {
			variadic = true
			continue
		}
		// A libc declaration usually names no parameter at all (`extern fn
		// strlen([*c]const u8) usize;`), sometimes qualified `noalias`; a
		// positional name stands in where the header gave none.
		p = strings.TrimPrefix(p, "noalias ")
		pname, ptyp, hasName := splitParam(p)
		if !hasName {
			pname, ptyp = fmt.Sprintf("a%d", i), p
		}
		if bareOpaqueByValue(ptyp, opaque) {
			return item{name: name, skip: "parameter `" + pname + "` of `" + name + "` passes `" + stripTagPrefix(strings.TrimSpace(ptyp)) + "` by value (opaque type; zig demoted it, most likely for a bitfield, so only a pointer to it crosses the C ABI)"}
		}
		kt, ok := mapByValueType(ptyp, known, opaque, abi)
		if !ok {
			return item{name: name, skip: "unsupported parameter type in `" + name + "`"}
		}
		params = append(params, sanitizeIdent(pname)+": "+kt)
	}
	ret := "Unit"
	if tail != "" {
		if bareOpaqueByValue(tail, opaque) {
			return item{name: name, skip: "`" + name + "` returns `" + stripTagPrefix(strings.TrimSpace(tail)) + "` by value (opaque type; zig demoted it, most likely for a bitfield, so only a pointer to it crosses the C ABI)"}
		}
		r, ok := mapByValueType(tail, known, opaque, abi)
		if !ok {
			return item{name: name, skip: "unsupported return type in `" + name + "`"}
		}
		ret = r
	}
	if variadic {
		params = append(params, "...")
	}
	return item{kind: "fn", name: name, text: "fn " + name + "(" + strings.Join(params, ", ") + ") -> " + ret}
}

// sanitizeIdent escapes a Kigumi keyword used as a C parameter name (`type`,
// `fn`, ...); zig's own translation already renames a Zig keyword the same
// way, so this only guards Kigumi's smaller, different keyword set.
func sanitizeIdent(name string) string {
	if kigumiKeywords[name] {
		return name + "_"
	}
	return name
}

var kigumiKeywords = map[string]bool{
	"fn": true, "let": true, "const": true, "mut": true, "type": true,
	"interface": true, "pub": true, "import": true, "if": true, "else": true,
	"match": true, "is": true, "for": true, "in": true, "return": true,
	"break": true, "continue": true, "defer": true, "errdefer": true,
	"fail": true, "pure": true, "noalloc": true, "async": true, "await": true,
	"unsafe": true, "move": true, "extern": true, "comptime": true,
	"true": true, "false": true, "self": true, "resource": true,
}
