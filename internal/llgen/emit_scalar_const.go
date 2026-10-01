package llgen

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

// scalarConst stores OpConst's literal straight into the register instead
// of round-tripping through rt_int/rt_float/rt_bool/rt_char.
func (e *emitter) scalarConst(sb *strings.Builder, in mir.Inst) bool {
	info, ok := e.scalar[in.Dst]
	if !ok {
		return false
	}
	e.storeNative(sb, in.Dst, Value{info.Ty, scalarConstText(e.r.Types, in)})
	return true
}

func scalarConstText(tt *sem.TypeTable, in mir.Inst) string {
	lit := in.Lit
	switch {
	case in.Str != "" && tt.IsInteger(in.Type):
		return in.Str
	case tt.IsFloat(in.Type):
		f := 0.0
		if lit.Float != nil {
			f, _ = lit.Float.Float64()
		} else if lit.Int != nil {
			f, _ = new(big.Float).SetInt(lit.Int).Float64()
		}
		return fmt.Sprintf("0x%X", math.Float64bits(f))
	case lit.Kind == sem.LitInt:
		if lit.Int == nil {
			return "0"
		}
		if lit.Int.IsInt64() {
			return lit.Int.String()
		}
		return fmt.Sprintf("%d", int64(lit.Int.Uint64()))
	case lit.Kind == sem.LitBool:
		if lit.Bool {
			return "1"
		}
		return "0"
	case lit.Kind == sem.LitChar:
		return strconv.Itoa(int(lit.Char))
	}
	return "0"
}
