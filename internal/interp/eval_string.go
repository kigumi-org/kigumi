package interp

import (
	"strings"

	"kigumi/internal/syntax"
)

func (fr *frame) stringLit(n syntax.NodeID) (Value, *ctrl) {
	parts := fr.t.StringParts(n)
	var sb strings.Builder
	src := fr.t.File.Src
	for _, p := range parts {
		if p.Expr == 0 {
			sb.WriteString(syntax.DecodeString(string(src[p.Text.Start:p.Text.End])))
			continue
		}
		v, c := fr.expr(p.Expr)
		if c != nil {
			return nil, c
		}
		sb.WriteString(display(v, fr.in))
	}
	return Str(sb.String()), nil
}
