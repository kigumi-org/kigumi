package llgen

import (
	"fmt"
	"strings"

	"kigumi/internal/sem"
)

func (e *emitter) asmTemplate(info *sem.AsmInfo, index map[int]int) string {
	defs := asmLocalLabelDefs(info.Template, info.Arch, info.Options["att_syntax"])
	var sb strings.Builder
	for i, line := range info.Template {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if stem, ok := asmLocalLabelDef(line, defs); ok {
			sb.WriteString(stem + "_${:uid}:")
			continue
		}
		pieces, _ := sem.AsmPieces(line)
		for _, p := range pieces {
			if p.Operand == -1 {
				sb.WriteString(asmRewriteRefText(strings.ReplaceAll(p.Text, "$", "$$"), defs))
				continue
			}
			op := info.Label(p.Text)
			n := index[op]
			mod := p.Modifier
			// x86 prints a symbol operand as `offset name` unless asked for
			// the bare name, which is what call and jmp need.
			if mod == "" && info.Operands[op].Kind == sem.AsmSym && (info.Arch == "amd64" || info.Arch == "386") {
				mod = "P"
			} else if m, ok := sem.AsmModifier(info.Arch, mod); ok {
				mod = m
			}
			if mod != "" {
				fmt.Fprintf(&sb, "${%d:%s}", n, mod)
			} else {
				fmt.Fprintf(&sb, "$%d", n)
			}
		}
	}
	return llQuote(sb.String())
}

func llQuote(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' || c < 0x20 || c > 0x7e {
			fmt.Fprintf(&sb, "\\%02X", c)
		} else {
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// asmAbiClobbers lists the C calling convention's caller-saved registers.
func asmAbiClobbers(arch string) []string {
	switch arch {
	case "amd64":
		regs := []string{"rax", "rcx", "rdx", "rsi", "rdi", "r8", "r9", "r10", "r11"}
		for i := 0; i < 32; i++ {
			regs = append(regs, fmt.Sprintf("xmm%d", i))
		}
		for i := 0; i < 8; i++ {
			regs = append(regs, fmt.Sprintf("k%d", i), fmt.Sprintf("mm%d", i))
		}
		return regs
	case "386":
		return []string{"eax", "ecx", "edx", "xmm0", "xmm1", "xmm2", "xmm3", "xmm4", "xmm5", "xmm6", "xmm7"}
	case "arm64":
		var regs []string
		for i := 0; i <= 17; i++ {
			regs = append(regs, fmt.Sprintf("x%d", i))
		}
		regs = append(regs, "x30")
		for i := 0; i <= 31; i++ {
			regs = append(regs, fmt.Sprintf("v%d", i))
		}
		for i := 0; i <= 15; i++ {
			regs = append(regs, fmt.Sprintf("p%d", i))
		}
		return append(regs, "ffr")
	case "arm":
		regs := []string{"r0", "r1", "r2", "r3", "r12", "lr"}
		for i := 0; i <= 15; i++ {
			regs = append(regs, fmt.Sprintf("s%d", i))
		}
		for i := 16; i <= 31; i++ {
			regs = append(regs, fmt.Sprintf("d%d", i))
		}
		return regs
	case "riscv64":
		regs := []string{"ra", "t0", "t1", "t2", "t3", "t4", "t5", "t6"}
		for i := 0; i <= 7; i++ {
			regs = append(regs, fmt.Sprintf("a%d", i))
		}
		for i := 0; i <= 11; i++ {
			regs = append(regs, fmt.Sprintf("ft%d", i))
		}
		for i := 0; i <= 7; i++ {
			regs = append(regs, fmt.Sprintf("fa%d", i))
		}
		for i := 0; i <= 31; i++ {
			regs = append(regs, fmt.Sprintf("v%d", i))
		}
		return regs
	}
	return nil
}
