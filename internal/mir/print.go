package mir

import (
	"fmt"
	"strings"
)

// Dump renders the program for golden tests.
func (p *Program) Dump() string {
	var sb strings.Builder
	for _, f := range p.Funcs {
		p.dumpFunc(&sb, f)
	}
	return sb.String()
}

func (p *Program) dumpFunc(sb *strings.Builder, f *Func) {
	fmt.Fprintf(sb, "fn %s(", f.Name)
	for i, prm := range f.Params {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(sb, "%%%s: %s", f.Locals[prm].Name, p.R.TypeString(f.Locals[prm].Type))
	}
	fmt.Fprintf(sb, ") -> %s {\n", p.R.TypeString(f.Ret))
	for bid, blk := range f.Blocks {
		fmt.Fprintf(sb, "  b%d:\n", bid)
		for _, in := range blk.Insts {
			sb.WriteString("    ")
			sb.WriteString(p.inst(f, in))
			sb.WriteByte('\n')
		}
		sb.WriteString("    ")
		sb.WriteString(p.term(f, blk.Term))
		sb.WriteByte('\n')
	}
	sb.WriteString("}\n")
}

func (p *Program) local(f *Func, l LocalID) string {
	if int(l) >= len(f.Locals) {
		return fmt.Sprintf("%%?%d", l)
	}
	return "%" + f.Locals[l].Name
}

func (p *Program) inst(f *Func, in Inst) string {
	var args []string
	for _, a := range in.Args {
		args = append(args, p.local(f, a))
	}
	head := ""
	switch in.Op {
	case OpDrop, OpSetField, OpSetIndex, OpCellSet, OpBoxReplace, OpPanic, OpNop:
	default:
		head = p.local(f, in.Dst) + " = "
	}
	body := in.Op.String()
	switch in.Op {
	case OpConst:
		body += " " + litText(in) + ": " + p.R.TypeString(in.Type)
	case OpCall, OpVariant, OpRecord, OpClosure, OpFnItem, OpBind, OpCFnPtr, OpIsVariant, OpIsType:
		body += " " + p.R.Entity(in.Ent).Name
		if in.Str != "" {
			body += " " + in.Str
		}
		if in.Async {
			body += " async"
		}
	case OpBinary, OpUnary, OpBuiltin:
		body += " " + in.Str
	case OpField, OpFieldMove, OpPayload, OpSetField, OpBorrow:
		body += fmt.Sprintf(" %d", in.Index)
	case OpBox:
		body += " " + p.R.TypeString(in.Type)
	case OpShare:
		if in.Copy {
			body += " copy"
		} else {
			body += " retain"
		}
	case OpInterp, OpShell:
		body += fmt.Sprintf(" %q", strings.Join(in.Strs, "\x00"))
	case OpPanic:
		body += " " + fmt.Sprintf("%q", in.Str)
	}
	if len(args) > 0 {
		body += "(" + strings.Join(args, ", ") + ")"
	}
	return head + body
}

func litText(in Inst) string {
	if in.Str != "" {
		return in.Str
	}
	l := in.Lit
	switch {
	case l.Int != nil:
		return l.Int.String()
	case l.Float != nil:
		return l.Float.Text('g', -1)
	case l.Kind == 3:
		return fmt.Sprintf("%q", l.Str)
	case l.Kind == 4:
		if l.Bool {
			return "true"
		}
		return "false"
	case l.Kind == 5:
		return fmt.Sprintf("%q", string(l.Char))
	case l.Kind == 6:
		return fmt.Sprintf("%q", l.Str)
	}
	return "0"
}

func (p *Program) term(f *Func, t Term) string {
	switch t.Op {
	case TermJump:
		return fmt.Sprintf("jump b%d", t.Targets[0])
	case TermBranch:
		return fmt.Sprintf("branch %s ? b%d : b%d", p.local(f, t.Args[0]), t.Targets[0], t.Targets[1])
	case TermReturn:
		return "return " + p.local(f, t.Args[0])
	case TermUnreachable:
		return "unreachable"
	}
	return "<no terminator>"
}
