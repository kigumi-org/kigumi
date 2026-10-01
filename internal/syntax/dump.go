package syntax

import (
	"fmt"
	"strings"

	"kigumi/internal/token"
)

// Dump renders the tree as indented text; the parser golden tests compare it.
func (t *Tree) Dump() string {
	d := &dumper{}
	t.dump(d, t.Root, "")
	return d.sb.String()
}

// dumper reuses one indent buffer instead of rebuilding it for every line.
type dumper struct {
	sb  strings.Builder
	ind []byte
}

func (d *dumper) push() { d.ind = append(d.ind, ' ', ' ') }
func (d *dumper) pop()  { d.ind = d.ind[:len(d.ind)-2] }

func (t *Tree) dump(d *dumper, id NodeID, label string) {
	d.sb.Write(d.ind)
	if label != "" {
		fmt.Fprintf(&d.sb, "%s: ", label)
	}
	if id == 0 {
		d.sb.WriteString("-\n")
		return
	}
	n := t.Nodes[id]
	d.sb.WriteString(n.Kind.String())
	switch n.Kind {
	case Ident, IntLit, FloatLit, CharLit, BoolLit, StringLit, ByteStringLit, MemberExpr,
		OptMemberExpr, PatBind, PatWildcard, FieldInit, NamedArg, PatField,
		Variant, Param, InstantiateDecl, Unary, Binary, AssignStmt,
		Field, PatRange, TypeLifetime:
		fmt.Fprintf(&d.sb, " %s", t.TokText(n.Tok))
	case GenericParam:
		fmt.Fprintf(&d.sb, " %s", t.TokText(n.Tok))
		switch {
		case n.Lhs&FlagConst != 0:
			d.sb.WriteString(" const")
		case n.Lhs&FlagLifetime != 0:
			d.sb.WriteString(" lifetime")
		}
	case ShellLit:
		fmt.Fprintf(&d.sb, " %s", t.TokText(n.Lhs))
	case TestDecl:
		fmt.Fprintf(&d.sb, " %s", t.TokText(n.Lhs))
	case Visibility:
		if n.Lhs != 0 {
			fmt.Fprintf(&d.sb, " %s", t.TokText(n.Lhs))
		}
	case AbiBlock, AsmAbi:
		fmt.Fprintf(&d.sb, " %s", t.TokText(n.Tok))
	case ContractExpr, ContractBlock:
		fmt.Fprintf(&d.sb, " %s", modsString(n.Lhs))
	case VariantField:
		fmt.Fprintf(&d.sb, " %s", t.TokText(n.Tok))
		if n.Lhs&FlagNamed != 0 {
			d.sb.WriteString(" named")
		}
	case TypeRef:
		if t.Toks[n.Tok].Kind == token.Lifetime {
			fmt.Fprintf(&d.sb, " %s", t.TokText(n.Tok))
		}
		if n.Lhs&FlagMut != 0 {
			d.sb.WriteString(" mut")
		}
	case BorrowExpr, TypePtr:
		if n.Lhs&FlagMut != 0 {
			d.sb.WriteString(" mut")
		}
	case Path:
		d.sb.WriteString(" ")
		for i, tk := range t.PathToks(id) {
			if i > 0 {
				d.sb.WriteString(".")
			}
			d.sb.WriteString(t.TokText(tk))
		}
	case ShellText:
		fmt.Fprintf(&d.sb, " %q", t.File.Src[n.Lhs:n.Rhs])
	case ShellRedirect:
		s := t.Slots(id)
		fmt.Fprintf(&d.sb, " %s fd=%d", redirNames[s[0]], s[1])
	}
	d.sb.WriteString("\n")
	t.dumpChildren(d, id)
}

func (t *Tree) dumpChildren(d *dumper, id NodeID) {
	n := t.Nodes[id]
	d.push()
	defer d.pop()
	child := func(c uint32) {
		if c != 0 || n.Kind == Binary || n.Kind == Unary || n.Kind == AssignStmt {
			t.dump(d, NodeID(c), "")
		}
	}
	switch shapes[n.Kind] {
	case sL:
		child(n.Lhs)
	case sLR:
		child(n.Lhs)
		child(n.Rhs)
	case sFlagR, sTokR:
		child(n.Rhs)
	case sList:
		for _, c := range t.Children(id) {
			t.dump(d, c, "")
		}
	case sRec:
		slots := t.Slots(id)
		for i, name := range recFields[n.Kind] {
			t.dumpSlot(d, n.Kind, name, slots[i])
		}
	}
}

func (t *Tree) dumpSlot(d *dumper, kind NodeKind, name string, v uint32) {
	switch name[0] {
	case '#':
		if v == 0 || kind == ShellRedirect {
			return
		}
		text := fmt.Sprint(v)
		switch name {
		case "#mods":
			text = modsString(v)
		case "#flags":
			text = flagsString(v)
		}
		d.sb.Write(d.ind)
		fmt.Fprintf(&d.sb, "%s: %s\n", name[1:], text)
	case '@':
		if v == 0 {
			return
		}
		text := t.TokText(v)
		if t.Toks[v].Kind == token.DocComment {
			text = fmt.Sprintf("%q", text)
		}
		d.sb.Write(d.ind)
		fmt.Fprintf(&d.sb, "%s: %s\n", name[1:], text)
	default:
		if v == 0 && kind != FnDecl {
			return
		}
		t.dump(d, NodeID(v), name)
	}
}

var redirNames = [...]string{"?", "<", ">", ">>", "<&", ">&"}

func modsString(m uint32) string {
	var parts []string
	switch {
	case m&ModPureVar != 0:
		parts = append(parts, "pure?")
	case m&ModPure != 0:
		parts = append(parts, "pure")
	}
	for _, e := range []struct {
		bit  uint32
		name string
	}{{ModNoalloc, "noalloc"}, {ModAsync, "async"}, {ModUnsafe, "unsafe"}} {
		if m&e.bit != 0 {
			parts = append(parts, e.name)
		}
	}
	return strings.Join(parts, " ")
}

func flagsString(f uint32) string {
	var parts []string
	for _, e := range []struct {
		bit  uint32
		name string
	}{{FlagMut, "mut"}, {FlagMove, "move"}, {FlagSelf, "self"}, {FlagVariadic, "variadic"}} {
		if f&e.bit != 0 {
			parts = append(parts, e.name)
		}
	}
	return strings.Join(parts, " ")
}
