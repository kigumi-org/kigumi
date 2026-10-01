package llgen

import (
	"fmt"
	"strings"
)

// stackSlot hoists to the entry block in a lowered function, since an
// alloca in a loop body would grow the stack each iteration; adapters and
// wrappers have no loops and take it inline.
func (e *emitter) stackSlot(sb *strings.Builder, spec string) string {
	t := e.newTmp()
	line := fmt.Sprintf("  %s = alloca %s\n", t, spec)
	if e.hoisting {
		e.entryAllocas = append(e.entryAllocas, line)
	} else {
		sb.WriteString(line)
	}
	return t
}

func (e *emitter) argArray(sb *strings.Builder, vals []string) string {
	n := len(vals)
	if n == 0 {
		n = 1
	}
	arr := e.stackSlot(sb, fmt.Sprintf("ptr, i32 %d", n))
	for i, v := range vals {
		slot := e.newTmp()
		fmt.Fprintf(sb, "  %s = getelementptr ptr, ptr %s, i32 %d\n", slot, arr, i)
		fmt.Fprintf(sb, "  store ptr %s, ptr %s\n", v, slot)
	}
	return arr
}
