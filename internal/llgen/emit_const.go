package llgen

import (
	"fmt"
	"math"
	"math/big"
	"strings"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

func (e *emitter) constant(sb *strings.Builder, in mir.Inst) string {
	lit := in.Lit
	tt := e.r.Types
	switch {
	case in.Str != "" && tt.IsInteger(in.Type):
		return e.irb(sb).Call(rtInt, vi64text(in.Str), vi32(e.numKind(in.Type))).Text
	case tt.IsFloat(in.Type):
		f := 0.0
		if lit.Float != nil {
			f, _ = lit.Float.Float64()
		} else if lit.Int != nil {
			f, _ = new(big.Float).SetInt(lit.Int).Float64()
		}
		return e.irb(sb).Call(rtFloat, vdouble(fmt.Sprintf("0x%X", math.Float64bits(f))), vi32(e.numKind(in.Type))).Text
	case lit.Kind == sem.LitInt:
		v := "0"
		if lit.Int != nil {
			if lit.Int.IsInt64() {
				v = lit.Int.String()
			} else {
				v = fmt.Sprintf("%d", int64(lit.Int.Uint64()))
			}
		}
		return e.irb(sb).Call(rtInt, vi64text(v), vi32(e.numKind(in.Type))).Text
	case lit.Kind == sem.LitBool:
		b := "0"
		if lit.Bool {
			b = "1"
		}
		return e.irb(sb).Call(rtBool, vi1(b)).Text
	case lit.Kind == sem.LitChar:
		return e.irb(sb).Call(rtChar, vi32(int(lit.Char))).Text
	case lit.Kind == sem.LitString:
		return e.irb(sb).Call(rtStr, vptr(e.cstr(lit.Str)), vi64(len(lit.Str))).Text
	case lit.Kind == sem.LitBytes:
		return e.irb(sb).Call(rtBytes, vptr(e.cstr(lit.Str)), vi64(len(lit.Str))).Text
	}
	return e.irb(sb).Call(rtUnit).Text
}

func (e *emitter) interp(sb *strings.Builder, in mir.Inst, args []string) string {
	var items []string
	ai := 0
	for _, s := range in.Strs {
		if s == "" {
			items = append(items, args[ai])
			ai++
			continue
		}
		items = append(items, e.irb(sb).Call(rtStr, vptr(e.cstr(s)), vi64(len(s))).Text)
	}
	arr := e.argArray(sb, items)
	return e.irb(sb).Call(rtInterp, vi32(len(items)), vptr(arr)).Text
}

// shell turns a `$"..."` literal into a std/shell.Plan, one
// std/shell.Item builder call per piece, in source order.
func (e *emitter) shell(sb *strings.Builder, in mir.Inst, args []string) string {
	plan := e.callStd(sb, "std/shell", "planNew", nil)
	ai := 0
	push := func(item string) { plan = e.callStd(sb, "std/shell", "planPush", []string{plan, item}) }
	for _, s := range in.Strs {
		switch {
		case s == "|":
			push(e.shellItem(sb, "Pipe", nil))
		case s == " ":
			push(e.shellItem(sb, "Word", nil))
		case s == "$":
			text := e.irb(sb).Call(rtDisplay, vptr(args[ai])).Text
			ai++
			push(e.shellItem(sb, "Text", []string{text}))
		case s[0] == 'r':
			op, fd, _ := strings.Cut(s[1:], ":")
			opv := e.irb(sb).Call(rtInt, vi64text(op), vi32(320)).Text
			fdv := e.irb(sb).Call(rtInt, vi64text(fd), vi32(320)).Text
			push(e.shellItem(sb, "Redirect", []string{opv, fdv}))
		default:
			text := s[1:]
			push(e.shellItem(sb, "Text", []string{e.irb(sb).Call(rtStr, vptr(e.cstr(text)), vi64(len(text))).Text}))
		}
	}
	return plan
}

func (e *emitter) shellItem(sb *strings.Builder, name string, payload []string) string {
	item := e.r.PackageMember(e.r.PackageByPath("std/shell"), "Item")
	for _, v := range e.r.TypeDecl(item).Variants {
		if e.r.Entity(v).Name == name {
			arr := e.argArray(sb, payload)
			return e.irb(sb).Call(rtVariant, vptr(e.vdesc(v)), vi64(len(payload)), vptr(arr)).Text
		}
	}
	panic("std/shell.Item has no variant " + name)
}

func (e *emitter) callStd(sb *strings.Builder, pkg, name string, args []string) string {
	fn := e.p.ByEnt[e.r.PackageMember(e.r.PackageByPath(pkg), name)]
	t := e.newTmp()
	fmt.Fprintf(sb, "  %s = call ptr %s(%s)\n", t, e.sym(fn), joinArgs(args))
	return t
}
