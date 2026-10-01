package llgen

import (
	"fmt"
	"strings"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

// asm lowers an asm block to an LLVM inline-asm call.
func (e *emitter) asm(sb *strings.Builder, f *mir.Func, in mir.Inst, args []string) string {
	info := e.r.AsmOf(f.File, in.Node)
	gpr, fpr, flags, _ := sem.AsmArch(info.Arch)
	var outs, cons, vals []string
	index := map[int]int{}
	for i, op := range info.Operands {
		if op.Kind == sem.AsmOut || op.Kind == sem.AsmInOut || op.Kind == sem.AsmLateOut {
			index[i] = len(outs)
			prefix := "=&"
			if op.Kind == sem.AsmLateOut {
				prefix = "="
			}
			cons = append(cons, prefix+e.asmConstraint(op, gpr, fpr))
			outs = append(outs, e.asmType(op.Type, info.Arch))
		}
	}
	argi := 0
	for i, op := range info.Operands {
		switch op.Kind {
		case sem.AsmIn, sem.AsmInOut:
			ct := e.asmType(op.Type, info.Arch)
			if op.Kind == sem.AsmIn {
				index[i] = len(outs) + len(vals)
				cons = append(cons, e.asmConstraint(op, gpr, fpr))
			} else {
				cons = append(cons, fmt.Sprint(index[i]))
			}
			vals = append(vals, ct+" "+e.unbox(sb, args[argi], op.Type, ct))
			argi++
		case sem.AsmConst:
			index[i] = len(outs) + len(vals)
			cons = append(cons, "i")
			vals = append(vals, fmt.Sprintf("i64 %d", op.Const))
		case sem.AsmSym:
			index[i] = len(outs) + len(vals)
			cons = append(cons, "s")
			vals = append(vals, "ptr "+e.cSymbol(op.Sym))
		}
	}
	for _, c := range info.Clobbers {
		cons = append(cons, "~{"+c+"}")
	}
	if info.ClobberAbi != "" {
		used := map[string]bool{}
		for _, op := range info.Operands {
			used[op.Reg] = true
		}
		for _, c := range asmAbiClobbers(info.Arch) {
			if !used[c] {
				cons = append(cons, "~{"+c+"}")
			}
		}
	}
	if !info.Options["nomem"] && !info.Options["readonly"] {
		cons = append(cons, "~{memory}")
	}
	if !info.Options["preserves_flags"] {
		for _, fl := range flags {
			cons = append(cons, "~{"+fl+"}")
		}
	}
	kw := ""
	if !info.Options["pure"] {
		kw += "sideeffect "
	}
	if (info.Arch == "amd64" || info.Arch == "386") && !info.Options["att_syntax"] {
		kw += "inteldialect "
	}
	tmpl := e.asmTemplate(info, index)
	ret := "void"
	switch len(outs) {
	case 0:
	case 1:
		ret = outs[0]
	default:
		ret = "{ " + strings.Join(outs, ", ") + " }"
	}
	if ret == "void" {
		fmt.Fprintf(sb, "  call void asm %s\"%s\", \"%s\"(%s)\n", kw, tmpl, strings.Join(cons, ","), strings.Join(vals, ", "))
		if info.Result == sem.TyNever {
			fmt.Fprintf(sb, "  unreachable\n")
		}
		return e.call(sb, "rt_unit", nil)
	}
	raw := e.newTmp()
	fmt.Fprintf(sb, "  %s = call %s asm %s\"%s\", \"%s\"(%s)\n", raw, ret, kw, tmpl, strings.Join(cons, ","), strings.Join(vals, ", "))
	return e.asmRecord(sb, info, raw, ret, outs)
}

func (e *emitter) asmRecord(sb *strings.Builder, info *sem.AsmInfo, raw, ret string, outs []string) string {
	fields := make([]string, len(outs))
	k := 0
	for _, op := range info.Operands {
		if op.Field < 0 {
			continue
		}
		v := raw
		if len(outs) > 1 {
			v = e.newTmp()
			fmt.Fprintf(sb, "  %s = extractvalue %s %s, %d\n", v, ret, raw, k)
		}
		fields[op.Field] = e.box(sb, v, op.Type, outs[k])
		k++
	}
	arr := e.argArray(sb, fields)
	return e.call(sb, "rt_record", []string{e.descOfType(info.Result), fmt.Sprintf("i64 %d", len(fields)), arr})
}

func (e *emitter) asmConstraint(op sem.AsmOperand, gpr, fpr string) string {
	if !op.Class {
		return "{" + op.Reg + "}"
	}
	if op.Reg == "freg" {
		return fpr
	}
	return gpr
}

func (e *emitter) asmType(t sem.TypeID, arch string) string {
	tt := e.r.Types
	ptr := 64
	if arch == "386" || arch == "arm" || arch == "wasm32" {
		ptr = 32
	}
	switch {
	case tt.IsFloat(t) && tt.Bits(t, ptr) == 32:
		return "float"
	case tt.IsFloat(t):
		return "double"
	case tt.IsInteger(t):
		return fmt.Sprintf("i%d", tt.Bits(t, ptr))
	}
	return "ptr"
}
