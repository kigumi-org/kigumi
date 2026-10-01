package llgen

import (
	"fmt"
	"strings"

	"kigumi/internal/mir"
)

// scalarBinary checks both operands are scalar-eligible; the MIR type
// checker never mixes register kinds within one binary op.
func (e *emitter) scalarBinary(sb *strings.Builder, f *mir.Func, in mir.Inst) bool {
	lty, lok := e.scalarType(f.Locals[in.Args[0]].Type)
	rty, rok := e.scalarType(f.Locals[in.Args[1]].Type)
	if !lok || !rok || lty != rty {
		return false
	}
	l, _ := e.scalarOperand(sb, f, in.Args[0])
	r, _ := e.scalarOperand(sb, f, in.Args[1])
	res, ok := e.nativeBinaryOp(sb, in.Str, lty, e.numKind(f.Locals[in.Args[0]].Type), l, r)
	if !ok {
		return false
	}
	e.storeScalarResult(sb, f, in.Dst, res)
	return true
}

// scalarUnary computes OpUnary's non-"cast" operators (!, -, ~) natively;
// cast is excluded upstream (classifyArgs and tryScalarInst).
func (e *emitter) scalarUnary(sb *strings.Builder, f *mir.Func, in mir.Inst) bool {
	ty, ok := e.scalarType(f.Locals[in.Args[0]].Type)
	if !ok {
		return false
	}
	v, _ := e.scalarOperand(sb, f, in.Args[0])
	nk := e.numKind(f.Locals[in.Args[0]].Type)
	var res Value
	switch {
	case ty == TI1 && in.Str == "!":
		res = Value{TI1, e.xorI1(sb, v.Text)}
	case ty == TI64 && in.Str == "-":
		res = Value{TI64, e.irb(sb).Call(rtIneg, v, vi32(nk)).Text}
	case ty == TI64 && in.Str == "~":
		res = Value{TI64, e.irb(sb).Call(rtInot, v, vi32(nk)).Text}
	case ty == TDouble && in.Str == "-":
		res = Value{TDouble, e.fneg(sb, v.Text)}
	default:
		return false
	}
	e.storeScalarResult(sb, f, in.Dst, res)
	return true
}

// nativeBinaryOp mirrors rt_binop's dispatch: "==" and "!=" work on every
// scalar kind, the rest need TI64 (mirrored by rt_i*) or TDouble.
func (e *emitter) nativeBinaryOp(sb *strings.Builder, op string, ty Type, nk int, l, r Value) (Value, bool) {
	if ty == TDouble {
		return e.nativeFloatOp(sb, op, l, r)
	}
	switch op {
	case "==":
		return Value{TI1, e.icmp(sb, "eq", ty, l, r)}, true
	case "!=":
		return Value{TI1, e.icmp(sb, "ne", ty, l, r)}, true
	}
	if ty != TI64 {
		return Value{}, false
	}
	signed := isSignedNk(nk)
	switch op {
	case "<", "<=", ">", ">=":
		return Value{TI1, e.icmp(sb, cmpCC(op, signed), ty, l, r)}, true
	case "+":
		return Value{TI64, e.irb(sb).Call(rtIadd, l, r, vi32(nk)).Text}, true
	case "-":
		return Value{TI64, e.irb(sb).Call(rtIsub, l, r, vi32(nk)).Text}, true
	case "*":
		return Value{TI64, e.irb(sb).Call(rtImul, l, r, vi32(nk)).Text}, true
	case "/":
		return Value{TI64, e.irb(sb).Call(rtIdiv, l, r, vi32(nk)).Text}, true
	case "%":
		return Value{TI64, e.irb(sb).Call(rtImod, l, r, vi32(nk)).Text}, true
	case "&":
		return Value{TI64, e.binOp(sb, "and", l, r)}, true
	case "|":
		return Value{TI64, e.binOp(sb, "or", l, r)}, true
	case "^":
		return Value{TI64, e.irb(sb).Call(rtIxor, l, r, vi32(nk)).Text}, true
	case "<<":
		return Value{TI64, e.irb(sb).Call(rtIshl, l, r, vi32(nk)).Text}, true
	case ">>":
		return Value{TI64, e.irb(sb).Call(rtIshr, l, r, vi32(nk)).Text}, true
	}
	return Value{}, false
}

// nativeFloatOp: rt_binop never checks a float result's bounds, so these
// are plain IEEE double instructions with no call at all.
func (e *emitter) nativeFloatOp(sb *strings.Builder, op string, l, r Value) (Value, bool) {
	switch op {
	case "==":
		return Value{TI1, e.fcmp(sb, "oeq", l, r)}, true
	case "!=":
		// rt_binop's "!=" is !(a==b) in C, true for NaN; "une" (not "one")
		// matches that since it treats unordered as not-equal.
		return Value{TI1, e.fcmp(sb, "une", l, r)}, true
	case "<":
		return Value{TI1, e.fcmp(sb, "olt", l, r)}, true
	case "<=":
		return Value{TI1, e.fcmp(sb, "ole", l, r)}, true
	case ">":
		return Value{TI1, e.fcmp(sb, "ogt", l, r)}, true
	case ">=":
		return Value{TI1, e.fcmp(sb, "oge", l, r)}, true
	case "+":
		return Value{TDouble, e.binOpT(sb, "fadd", TDouble, l, r)}, true
	case "-":
		return Value{TDouble, e.binOpT(sb, "fsub", TDouble, l, r)}, true
	case "*":
		return Value{TDouble, e.binOpT(sb, "fmul", TDouble, l, r)}, true
	case "/":
		return Value{TDouble, e.binOpT(sb, "fdiv", TDouble, l, r)}, true
	}
	return Value{}, false
}

// isSignedNk reads mir.NumKind's sign bit (bit 8).
func isSignedNk(nk int) bool { return nk&(1<<8) != 0 }

func cmpCC(op string, signed bool) string {
	table := map[string][2]string{
		"<":  {"slt", "ult"},
		"<=": {"sle", "ule"},
		">":  {"sgt", "ugt"},
		">=": {"sge", "uge"},
	}
	cc := table[op]
	if signed {
		return cc[0]
	}
	return cc[1]
}

func (e *emitter) xorI1(sb *strings.Builder, x string) string {
	t := e.newTmp()
	fmt.Fprintf(sb, "  %s = xor i1 %s, true\n", t, x)
	return t
}

func (e *emitter) fneg(sb *strings.Builder, x string) string {
	t := e.newTmp()
	fmt.Fprintf(sb, "  %s = fneg double %s\n", t, x)
	return t
}

func (e *emitter) binOp(sb *strings.Builder, op string, l, r Value) string {
	return e.binOpT(sb, op, TI64, l, r)
}

func (e *emitter) binOpT(sb *strings.Builder, op string, ty Type, l, r Value) string {
	t := e.newTmp()
	fmt.Fprintf(sb, "  %s = %s %s %s, %s\n", t, op, ty, l.Text, r.Text)
	return t
}

func (e *emitter) icmp(sb *strings.Builder, cc string, ty Type, l, r Value) string {
	t := e.newTmp()
	fmt.Fprintf(sb, "  %s = icmp %s %s %s, %s\n", t, cc, ty, l.Text, r.Text)
	return t
}

func (e *emitter) fcmp(sb *strings.Builder, cc string, l, r Value) string {
	t := e.newTmp()
	fmt.Fprintf(sb, "  %s = fcmp %s double %s, %s\n", t, cc, l.Text, r.Text)
	return t
}
