package llgen

import (
	"fmt"
	"strconv"
	"strings"

	"kigumi/internal/mir"
	"kigumi/internal/sem"
)

// nakedFn emits an export(C, naked) function as module-level assembly text;
// no LLVM function exists for it.
func (e *emitter) nakedFn(f *mir.Func) {
	var info *sem.AsmInfo
	for _, b := range f.Blocks {
		for _, in := range b.Insts {
			if in.Op == mir.OpAsm {
				info = e.r.AsmOf(f.File, in.Node)
			}
		}
	}
	if info == nil {
		return
	}
	name := e.symPrefix + e.r.Entity(f.Ent).Name
	e.declareForeign(f.Ent)
	intel := (info.Arch == "amd64" || info.Arch == "386") && !info.Options["att_syntax"]
	lines := []string{".text", ".globl " + name}
	if intel {
		lines = append(lines, ".intel_syntax noprefix")
	}
	lines = append(lines, name+":")
	for _, line := range info.Template {
		lines = append(lines, e.nakedLine(info, line))
	}
	if intel {
		lines = append(lines, ".att_syntax prefix")
	}
	for _, l := range lines {
		fmt.Fprintf(&e.modAsm, "module asm \"%s\"\n", llQuote(l))
	}
	e.modAsm.WriteString("\n")
}

func (e *emitter) nakedLine(info *sem.AsmInfo, line string) string {
	pieces, _ := sem.AsmPieces(line)
	var sb strings.Builder
	for _, p := range pieces {
		if p.Operand == -1 {
			sb.WriteString(p.Text)
			continue
		}
		op := info.Operands[info.Label(p.Text)]
		switch op.Kind {
		case sem.AsmConst:
			sb.WriteString(strconv.FormatInt(op.Const, 10))
		case sem.AsmSym:
			sb.WriteString(e.symPrefix + strings.Trim(strings.TrimPrefix(e.cSymbol(op.Sym), "@"), "\""))
		}
	}
	return sb.String()
}
