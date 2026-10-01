package llgen

import "fmt"

// capSuffix distinguishes cReader/cWriter variants compiled at different
// alignments, so each stays correctly annotated.
func capSuffix(cap int64) string {
	if cap <= 0 {
		return ""
	}
	return fmt.Sprintf("_a%d", cap)
}

// capAligns lowers each alignment to at most cap, in place; cap <= 0 is a no-op.
func capAligns(aligns []int64, cap int64) {
	if cap <= 0 {
		return
	}
	for i, a := range aligns {
		aligns[i] = min(a, cap)
	}
}
