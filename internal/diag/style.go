package diag

// Colors is whether rendered diagnostics carry ANSI colors. The CLI turns
// it on once for the process when stderr is a terminal (spec: NO_COLOR and
// --color decide); everything else renders plain text.
var colors bool

// UseColor switches colored rendering on or off.
func UseColor(on bool) { colors = on }

// The palette follows rustc: the severity word in its own color, the
// location arrow and gutter in blue, help in cyan, everything bold that a
// reader scans for.
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiRed    = "\x1b[1;31m"
	ansiYellow = "\x1b[1;33m"
	ansiGreen  = "\x1b[1;32m"
	ansiCyan   = "\x1b[1;36m"
	ansiBlue   = "\x1b[1;34m"
)

func paint(code, text string) string {
	if !colors {
		return text
	}
	return code + text + ansiReset
}

func severityColor(s Severity) string {
	switch s {
	case Error:
		return ansiRed
	case Warning:
		return ansiYellow
	}
	return ansiGreen
}

// underlineColor marks a primary span in the severity's color and a
// note's secondary span in blue, so the eye finds the cause first.
func underlineColor(s Severity) string {
	if s == Note {
		return ansiBlue
	}
	return severityColor(s)
}

// Prefix colors a runtime prefix such as `panic` or `error` the way the
// same severity is colored in a diagnostic.
func Prefix(word string) string {
	switch word {
	case "error", "panic":
		return paint(ansiRed, word)
	case "warning":
		return paint(ansiYellow, word)
	}
	return paint(ansiBold, word)
}
