package sem

import "kigumi/internal/syntax"

// Output types for out/inout/lateout operands are settled later, by asmResult.
func (c *checker) asmOperand(line syntax.NodeID, arch *asmArch, labels map[string]syntax.NodeID) AsmOperand {
	s := c.t.Slots(line)
	op := AsmOperand{Field: -1, Expr: syntax.NodeID(s[3]), Node: line}
	if s[0] != 0 {
		op.Label = c.t.TokText(s[0])
		if _, dup := labels[op.Label]; dup {
			c.errAt(line, cAsmLabelDup, op.Label)
		}
		labels[op.Label] = line
	}
	switch kind := c.t.TokText(s[1]); kind {
	case "in":
		op.Kind = AsmIn
	case "out":
		op.Kind = AsmOut
	case "inout":
		op.Kind = AsmInOut
	case "lateout":
		op.Kind = AsmLateOut
	case "const":
		op.Kind = AsmConst
		c.asmConst(&op)
	case "sym":
		op.Kind = AsmSym
		c.asmSym(&op)
	}
	if op.Kind == AsmConst || op.Kind == AsmSym {
		if op.Label == "" {
			c.errAt(line, cAsmLabelRequired, op.Kind.String())
		}
		return op
	}
	if s[2] != 0 {
		op.Reg = c.t.TokText(s[2])
	}
	if op.Reg == "reg" || op.Reg == "freg" {
		op.Class = true
	} else if r, ok := arch.Regs[op.Reg]; !ok {
		c.errAt(line, cAsmRegister, op.Reg, c.r.arch)
	} else if r.Reserved != "" {
		c.errAt(line, cAsmRegisterReserved, op.Reg, r.Reserved)
	}
	switch {
	case op.Kind == AsmIn && op.Expr == 0:
		c.errAt(line, cAsmValue, "`in` needs a value")
	case op.Kind == AsmInOut && op.Expr == 0:
		c.errAt(line, cAsmValue, "`inout` needs the input value; the result field receives the output")
	case (op.Kind == AsmOut || op.Kind == AsmLateOut) && op.Expr != 0:
		c.errAt(line, cAsmValue, "`"+op.Kind.String()+"` takes no value; the result record field receives it")
		c.synth(op.Expr)
	case op.Kind != AsmIn && op.Label == "":
		c.errAt(line, cAsmLabelRequired, op.Kind.String())
	}
	if op.Kind == AsmIn && op.Expr != 0 {
		op.Type = c.asmInput(&op, arch)
		c.asmOperandType(&op, arch)
	}
	return op
}

// An untyped literal takes the register's natural integer width.
func (c *checker) asmInput(op *AsmOperand, arch *asmArch) TypeID {
	t := c.readThrough(c.vars.resolve(c.synth(op.Expr)))
	if c.r.Types.Kind(t) != KUntyped {
		return t
	}
	bits := arch.PtrBits
	if r, ok := arch.Regs[op.Reg]; ok && !r.Float {
		bits = r.Bits
	}
	natural := map[int]TypeID{8: TyI8, 16: TyI16, 32: TyI32, 64: TyI64}[bits]
	if natural == 0 || c.r.Types.IsFloat(defaultOf(t)) {
		natural = defaultOf(t)
	}
	return c.adopt(c.literalNode(op.Expr), natural)
}

func (c *checker) asmOperandType(op *AsmOperand, arch *asmArch) {
	tt := c.r.Types
	t := op.Type
	if t == TyPoison || t == 0 {
		return
	}
	isPtr := tt.Kind(t) == KPtr || tt.IsCFn(t)
	if !tt.IsNumeric(t) && !isPtr {
		c.errAt(op.Node, cAsmOperandType, c.r.TypeString(t))
		return
	}
	bits := arch.PtrBits
	if !isPtr {
		bits = tt.Bits(t, arch.PtrBits)
	}
	r, explicit := arch.Regs[op.Reg]
	switch {
	case op.Class && op.Reg == "freg" && !tt.IsFloat(t), explicit && r.Float && !tt.IsFloat(t):
		c.errAt(op.Node, cAsmWidth, c.r.TypeString(t), "a floating-point register")
	case op.Class && op.Reg == "reg" && tt.IsFloat(t):
		c.errAt(op.Node, cAsmWidth, c.r.TypeString(t), "`reg`; use `freg`")
	case op.Class && bits > arch.PtrBits:
		c.errAt(op.Node, cAsmWidth, c.r.TypeString(t), "a general register")
	case explicit && !r.Float && bits != r.Bits:
		c.errAt(op.Node, cAsmWidth, c.r.TypeString(t), "`"+op.Reg+"`, which is "+itoa(r.Bits)+" bits")
	case explicit && r.Float && bits > r.Bits:
		c.errAt(op.Node, cAsmWidth, c.r.TypeString(t), "`"+op.Reg+"`, which is "+itoa(r.Bits)+" bits")
	}
}

func (c *checker) asmConst(op *AsmOperand) {
	if op.Expr == 0 {
		c.errAt(op.Node, cAsmConst)
		return
	}
	t := c.vars.resolve(c.synth(op.Expr))
	if c.r.Types.Kind(t) == KUntyped {
		t = c.adopt(c.literalNode(op.Expr), TyI64)
	}
	op.Type = t
	if lit, ok := c.r.LiteralOf(c.t, c.literalNode(op.Expr)); ok && lit.Kind == LitInt && lit.Int != nil && lit.Int.IsInt64() {
		op.Const = lit.Int.Int64()
		return
	}
	if ent := c.info.Uses[op.Expr]; ent != 0 && c.r.Entities[ent].Kind == EntConst {
		if lit, ok := c.r.ConstValueOf(ent); ok && lit.Kind == LitInt && lit.Int != nil && lit.Int.IsInt64() {
			op.Const = lit.Int.Int64()
			return
		}
	}
	c.errAt(op.Node, cAsmConst)
}

func (c *checker) asmSym(op *AsmOperand) {
	if op.Expr != 0 && (c.t.Kind(op.Expr) == syntax.Ident || c.t.Kind(op.Expr) == syntax.MemberExpr) {
		if h := c.resolveHead(op.Expr); h.kind == headFn {
			info := c.r.Fn(h.ent)
			if info.Abi != "" || info.Export != "" {
				c.r.useEntity(c.f, op.Expr, h.ent)
				c.info.Uses[op.Expr] = h.ent
				op.Sym = h.ent
				return
			}
		}
	}
	c.errAt(op.Node, cAsmSym)
	if op.Expr != 0 && c.t.Kind(op.Expr) != syntax.Ident && c.t.Kind(op.Expr) != syntax.MemberExpr {
		c.synth(op.Expr)
	}
}
