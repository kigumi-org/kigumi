package cheader

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var reRecord = regexp.MustCompile(`(?s)^pub const (\w+) = extern (struct|union) \{(.*)\}$`)
var reOpaqueName = regexp.MustCompile(`^pub const (\w+) = opaque \{`)

func classifyRecord(chunk string, known, opaque map[string]bool, abi ABI) item {
	m := reRecord.FindStringSubmatch(chunk)
	name, kind, body := m[1], m[2], strings.TrimSpace(m[3])
	if kind == "union" {
		return item{name: name, skip: "C union (Kigumi has no ABI-safe union type)"}
	}
	body = dropNamespaceMembers(body)
	if body == "" {
		return registerOpaque(name, known, opaque)
	}
	var fields []string
	for _, raw := range splitTopCommas(body) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		fname, rest, ok := splitParam(raw)
		if !ok {
			return item{name: name, skip: "unsupported field in `" + name + "`"}
		}
		ftyp, _, _ := strings.Cut(rest, "=")
		ftyp = strings.TrimSpace(ftyp)
		if bareOpaqueByValue(ftyp, opaque) {
			return item{name: name, skip: "field `" + fname + "` of `" + name + "` holds `" + stripTagPrefix(ftyp) + "` by value (opaque type; zig demoted it, most likely for a bitfield, so only a pointer to it crosses the C ABI)"}
		}
		kt, ok := mapByValueType(ftyp, known, opaque, abi)
		if !ok {
			return item{name: name, skip: "unsupported field `" + fname + "` in `" + name + "`"}
		}
		fields = append(fields, "    pub "+sanitizeIdent(fname)+" "+kt)
	}
	friendly := stripTagPrefix(name)
	known[friendly] = true
	text := "type " + friendly + " layout(C) = {\n" + strings.Join(fields, "\n") + "\n}"
	return item{kind: "record", name: friendly, text: text}
}

// dropNamespaceMembers removes the `pub const f = __root.f;` lines zig adds
// inside a struct body for a function whose first parameter is that struct
// (a UFCS-style convenience Kigumi has no use for); a real field's line
// never starts with `pub`, so this is enough to tell the two apart.
func dropNamespaceMembers(body string) string {
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "pub ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// widenEnumMembers retypes a run of enum member consts (pending, indices
// into consts) to scalar: zig gives each member its own type independently
// of the enum's, so a run of `c_int` members can sit right before an
// `enum_X = c_uint` alias (every member non-negative) and disagree with it.
func widenEnumMembers(consts []item, pending []int, scalar string) {
	for _, i := range pending {
		if consts[i].scalar == scalar {
			continue
		}
		consts[i].scalar = scalar
		consts[i].text = "const " + consts[i].name + ": " + scalar + " = " + consts[i].lit
	}
}

func registerOpaque(name string, known, opaque map[string]bool) item {
	friendly := stripTagPrefix(name)
	known[friendly] = true
	opaque[friendly] = true
	return item{kind: "opaque", name: friendly, text: "type " + friendly}
}

func classifyOpaque(chunk string, known, opaque map[string]bool) item {
	m := reOpaqueName.FindStringSubmatch(chunk)
	return registerOpaque(m[1], known, opaque)
}

// Generate turns a header's zig translate-c output (full) into Kigumi
// source, using the same tool's output on an empty header (baseline) to
// tell the header's own declarations from zig's built-in predefines.
// skipped lists what was left out, one entry per declaration, sorted and
// deduplicated by name.
func Generate(full, baseline string, abi ABI) (source string, skipped []string) {
	seen := map[string]bool{}
	for _, c := range splitTopLevel(baseline) {
		if k, n, ok := declHead(c); ok {
			seen[k+" "+n] = true
		}
	}
	known := map[string]bool{}
	opaque := map[string]bool{}
	var opaques, records, aliases, fns, consts []item
	var skip []string
	var pendingMembers []int
	for _, c := range splitTopLevel(full) {
		k, n, ok := declHead(c)
		if !ok || seen[k+" "+n] {
			continue
		}
		it := classify(c, known, opaque, abi)
		switch {
		case it.kind == "noop":
		case it.skip != "":
			skip = append(skip, it.name+": "+it.skip)
			pendingMembers = nil
		case it.kind == "opaque":
			opaques = append(opaques, it)
			pendingMembers = nil
		case it.kind == "record":
			records = append(records, it)
			pendingMembers = nil
		case it.kind == "alias":
			if it.scalar != "" {
				widenEnumMembers(consts, pendingMembers, it.scalar)
			}
			aliases = append(aliases, it)
			pendingMembers = nil
		case it.kind == "fn":
			fns = append(fns, it)
			pendingMembers = nil
		case it.kind == "const":
			consts = append(consts, it)
			if it.scalar != "" {
				pendingMembers = append(pendingMembers, len(consts)-1)
			} else {
				pendingMembers = nil
			}
		}
	}
	byName := func(s []item) { sort.Slice(s, func(i, j int) bool { return s[i].name < s[j].name }) }
	byName(opaques)
	byName(records)
	byName(aliases)
	byName(fns)
	byName(consts)
	if n := strings.Count(full, "demoted to opaque type"); n > 0 {
		skip = append(skip, fmt.Sprintf("%d struct(s) use bitfields or another feature Kigumi can't lay out; imported as opaque types (fields inaccessible)", n))
	}
	sort.Strings(skip)
	return render(opaques, records, aliases, fns, consts), dedupe(skip)
}

func render(opaques, records, aliases, fns, consts []item) string {
	var b strings.Builder
	if len(opaques) > 0 || len(fns) > 0 {
		b.WriteString("extern(C) {\n")
		for _, t := range opaques {
			fmt.Fprintf(&b, "    pub %s\n", t.text)
		}
		if len(opaques) > 0 && len(fns) > 0 {
			b.WriteString("\n")
		}
		for _, f := range fns {
			fmt.Fprintf(&b, "    pub %s\n", f.text)
		}
		b.WriteString("}\n")
	}
	for _, group := range [][]item{records, aliases, consts} {
		for _, it := range group {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "pub %s\n", it.text)
		}
	}
	return b.String()
}

func dedupe(s []string) []string {
	var out []string
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}
