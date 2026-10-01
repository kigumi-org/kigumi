package llgen

import (
	"fmt"
	"strings"
)

func (e *emitter) ptrTable(name string, items []string) string {
	e.tables.WriteString(e.ptrTableDef(name, items))
	return name
}

func (e *emitter) ptrTableDef(name string, items []string) string {
	var parts []string
	for _, it := range items {
		parts = append(parts, "ptr "+it)
	}
	if len(parts) == 0 {
		parts = append(parts, "ptr null")
	}
	return fmt.Sprintf("%s = global [%d x ptr] [%s]\n", name, len(parts), strings.Join(parts, ", "))
}
