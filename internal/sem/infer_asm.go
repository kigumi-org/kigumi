package sem

import (
	"kigumi/internal/syntax"
)

var asmOptionNames = map[string]bool{"nomem": true, "readonly": true, "pure": true, "nostack": true, "preserves_flags": true, "noreturn": true, "att_syntax": true}

func (c *checker) synthAsm(n syntax.NodeID, want TypeID) TypeID {
	if c.unsafe == 0 && !c.naked {
		c.errAt(n, cUnsafeRequired, "inline assembly")
	}
	arch, ok := asmArches[c.r.arch]
	if !ok {
		c.errAt(n, cAsmTarget, c.r.arch)
		return TyPoison
	}
	c.callInvalidates()
	info := &AsmInfo{Arch: c.r.arch, Options: map[string]bool{}}
	var outputs []int
	var templates []syntax.NodeID
	labels := map[string]syntax.NodeID{}
	for _, line := range c.t.Children(syntax.NodeID(c.t.Nodes[n].Lhs)) {
		switch c.t.Kind(line) {
		case syntax.AsmTemplate:
			templates = append(templates, line)
			c.asmTemplate(line, info)
		case syntax.AsmOperand:
			op := c.asmOperand(line, arch, labels)
			if c.naked && op.Kind != AsmConst && op.Kind != AsmSym {
				c.errAt(line, cNakedOperand)
			}
			if op.Kind == AsmOut || op.Kind == AsmInOut || op.Kind == AsmLateOut {
				outputs = append(outputs, len(info.Operands))
			}
			info.Operands = append(info.Operands, op)
		case syntax.AsmClobber:
			c.asmClobbers(line, arch, info)
		case syntax.AsmOptions:
			c.asmOptions(line, info)
		}
	}
	c.asmPlaceholders(templates, info)
	if info.Options["pure"] && !info.Options["nomem"] && !info.Options["readonly"] {
		c.errAt(n, cAsmOptionConflict, "`pure` needs `nomem` or `readonly`")
	}
	if info.Options["pure"] && len(outputs) == 0 {
		c.errAt(n, cAsmOptionConflict, "`pure` asm needs an output")
	}
	if info.Options["att_syntax"] && c.r.arch != "amd64" && c.r.arch != "386" {
		c.errAt(n, cAsmOptionConflict, "`att_syntax` is an x86 option")
	}
	switch {
	case c.naked:
		// The whole function is the asm; its signature is the C contract.
		info.Result = c.retType
	case info.Options["noreturn"]:
		if len(outputs) > 0 {
			c.errAt(n, cAsmOptionConflict, "`noreturn` asm cannot have outputs")
		}
		info.Result = TyNever
	case len(outputs) == 0:
		info.Result = TyUnit
	default:
		info.Result = c.asmResult(n, want, info, outputs)
	}
	c.info.Asm[n] = info
	return info.Result
}

func (c *checker) asmTemplate(line syntax.NodeID, info *AsmInfo) {
	lit := syntax.NodeID(c.t.Nodes[line].Lhs)
	text := ""
	for _, p := range c.t.StringParts(lit) {
		if p.Expr != 0 {
			c.errAt(line, cAsmInterpolation)
			c.synth(p.Expr)
			continue
		}
		text += syntax.DecodeString(string(c.t.File.Src[p.Text.Start:p.Text.End]))
	}
	if _, err := AsmPieces(text); err != "" {
		c.errAt(line, cAsmTemplateBrace, err)
	}
	info.Template = append(info.Template, text)
}

func (c *checker) asmPlaceholders(templates []syntax.NodeID, info *AsmInfo) {
	for i, line := range info.Template {
		pieces, _ := AsmPieces(line)
		for _, p := range pieces {
			if p.Operand != -2 {
				continue
			}
			if info.Label(p.Text) < 0 {
				c.errAt(templates[i], cAsmPlaceholder, p.Text)
			} else if _, ok := asmModifiers[c.r.arch][p.Modifier]; p.Modifier != "" && !ok {
				c.errAt(templates[i], cAsmModifier, p.Modifier, c.r.arch)
			}
		}
	}
}

func (c *checker) asmClobbers(line syntax.NodeID, arch *asmArch, info *AsmInfo) {
	for _, it := range c.t.Children(syntax.NodeID(c.t.Nodes[line].Lhs)) {
		name := c.t.TokText(c.t.Nodes[it].Tok)
		if c.t.Kind(it) == syntax.AsmAbi {
			if name != "C" {
				c.errAt(it, cAbiUnknown, name)
			}
			info.ClobberAbi = name
			continue
		}
		if _, ok := arch.Regs[name]; !ok {
			c.errAt(it, cAsmClobber, name)
			continue
		}
		info.Clobbers = append(info.Clobbers, name)
	}
}

func (c *checker) asmOptions(line syntax.NodeID, info *AsmInfo) {
	for _, it := range c.t.Children(syntax.NodeID(c.t.Nodes[line].Lhs)) {
		name := c.t.TokText(c.t.Nodes[it].Tok)
		switch {
		case !asmOptionNames[name]:
			c.errAt(it, cAsmOption, name)
		case info.Options[name]:
			c.errAt(it, cAsmOptionConflict, "`"+name+"` is given twice")
		default:
			info.Options[name] = true
		}
	}
}

func (c *checker) asmResult(n syntax.NodeID, want TypeID, info *AsmInfo, outputs []int) TypeID {
	tt := c.r.Types
	w := c.vars.resolve(want)
	if elem, ok := tt.IsOption(w); ok {
		w = c.vars.resolve(elem)
	} else if val, _, ok := tt.IsResult(w); ok {
		w = c.vars.resolve(val)
	}
	if tt.Kind(w) != KNamed || c.r.Entities[tt.Node(w).Ent].File == 0 || c.r.typeDecl(tt.Node(w).Ent).Form != FormRecord {
		if w != TyPoison {
			c.errAt(n, cAsmResultType)
		}
		return TyPoison
	}
	decl := c.r.typeDecl(tt.Node(w).Ent)
	subst := map[EntityID]TypeID{}
	for i, p := range decl.Params {
		subst[p] = tt.Node(w).Args[i]
	}
	written := map[EntityID]bool{}
	for _, i := range outputs {
		op := &info.Operands[i]
		fld := c.r.findField(tt.Node(w).Ent, op.Label)
		if fld == 0 {
			c.errAt(op.Node, cAsmFieldUnknown, c.r.TypeString(w), op.Label)
			continue
		}
		if written[fld] {
			c.errAt(op.Node, cAsmLabelDup, op.Label)
			continue
		}
		written[fld] = true
		for j, f := range decl.Fields {
			if f == fld {
				op.Field = j
			}
		}
		op.Type = c.vars.resolve(tt.Subst(c.r.Entities[fld].Type, subst))
		c.asmOperandType(op, asmArches[c.r.arch])
		if op.Kind == AsmInOut && op.Expr != 0 {
			c.check(op.Expr, op.Type)
		}
	}
	for _, f := range decl.Fields {
		if !written[f] {
			c.errAt(n, cAsmFieldMissing, c.r.Entities[f].Name, c.r.TypeString(w))
		}
	}
	return w
}
