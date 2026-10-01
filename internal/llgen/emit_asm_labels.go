package llgen

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	asmNumericDefRe = regexp.MustCompile(`^[0-9]+:$`)
	asmNumericRefRe = regexp.MustCompile(`\b([0-9]+)[bf]\b`)
)

// Intel syntax reads `1b` as a binary literal, not a label (AT&T has no
// such ambiguity), so each block's labels are renamed to stay unambiguous.
func asmLocalLabelDefs(lines []string, arch string, attSyntax bool) map[string]string {
	if attSyntax || (arch != "amd64" && arch != "386") {
		return nil
	}
	var defs map[string]string
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if !asmNumericDefRe.MatchString(t) {
			continue
		}
		n := t[:len(t)-1]
		if defs == nil {
			defs = map[string]string{}
		}
		if _, ok := defs[n]; !ok {
			defs[n] = fmt.Sprintf("kg_l%s", n)
		}
	}
	return defs
}

// ${:uid} is LLVM's per-instantiation counter, kept for uniqueness. A
// number with no matching definition is a literal immediate, left as is.
func asmRewriteRefText(text string, defs map[string]string) string {
	if defs == nil {
		return text
	}
	return asmNumericRefRe.ReplaceAllStringFunc(text, func(m string) string {
		stem, ok := defs[m[:len(m)-1]]
		if !ok {
			return m
		}
		return stem + "_${:uid}"
	})
}

func asmLocalLabelDef(line string, defs map[string]string) (string, bool) {
	if defs == nil {
		return "", false
	}
	t := strings.TrimSpace(line)
	if !asmNumericDefRe.MatchString(t) {
		return "", false
	}
	stem, ok := defs[t[:len(t)-1]]
	return stem, ok
}
