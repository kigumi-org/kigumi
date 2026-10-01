package llgen

import (
	_ "embed"
	"strings"
)

//go:embed runtime/freestanding_symbols.txt
var freestandingSymbolsTxt string

// FreestandingSymbols is the allow-list of symbols a `--sys none` archive
// may leave undefined (internal/driver checks real archives against it).
func FreestandingSymbols() map[string]bool {
	out := map[string]bool{}
	for line := range strings.Lines(freestandingSymbolsTxt) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	return out
}
