package sem

import "kigumi/internal/syntax"

type AsmOperandKind uint8

const (
	AsmIn AsmOperandKind = iota + 1
	AsmOut
	AsmInOut
	AsmLateOut
	AsmConst
	AsmSym
)

func (k AsmOperandKind) String() string {
	return [...]string{"", "in", "out", "inout", "lateout", "const", "sym"}[k]
}

// AsmOperand is one operand line of an asm block after checking.
type AsmOperand struct {
	Kind  AsmOperandKind
	Label string
	// Reg is the explicit register, or the class name when Class is set.
	Reg   string
	Class bool
	Type  TypeID
	// Field indexes the result record for outputs, -1 otherwise.
	Field int
	// Expr is the input value of in/inout, or the const or sym expression.
	Expr  syntax.NodeID
	Const int64
	Sym   EntityID
	Node  syntax.NodeID
}

// AsmInfo is the checked form of an asm block.
type AsmInfo struct {
	Arch       string
	Template   []string
	Operands   []AsmOperand
	Clobbers   []string
	ClobberAbi string
	Options    map[string]bool
	// Result is the record the outputs fill, Unit without outputs, or
	// Never for noreturn.
	Result TypeID
}

// Label returns the operand index a template placeholder names, or -1.
func (a *AsmInfo) Label(name string) int {
	for i, op := range a.Operands {
		if op.Label == name {
			return i
		}
	}
	return -1
}

// AsmPiece is a run of template text (Operand < 0) or a placeholder.
type AsmPiece struct {
	Text     string
	Operand  int
	Modifier string
}

// Labels are left unresolved (Operand = -2, Text = label) for the caller to map.
func AsmPieces(line string) (pieces []AsmPiece, err string) {
	text := ""
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '{' && i+1 < len(line) && line[i+1] == '{':
			text += "{"
			i++
		case c == '}' && i+1 < len(line) && line[i+1] == '}':
			text += "}"
			i++
		case c == '{':
			end := i + 1
			for end < len(line) && line[end] != '}' {
				end++
			}
			if end == len(line) {
				return nil, "unbalanced `{`; write `{{` for a literal brace"
			}
			name, mod := line[i+1:end], ""
			if j := indexByte(name, ':'); j >= 0 {
				name, mod = name[:j], name[j+1:]
			}
			if name == "" {
				return nil, "empty placeholder `{}`"
			}
			if text != "" {
				pieces = append(pieces, AsmPiece{Text: text, Operand: -1})
				text = ""
			}
			pieces = append(pieces, AsmPiece{Text: name, Operand: -2, Modifier: mod})
			i = end
		case c == '}':
			return nil, "unbalanced `}`; write `}}` for a literal brace"
		default:
			text += string(c)
		}
	}
	if text != "" {
		pieces = append(pieces, AsmPiece{Text: text, Operand: -1})
	}
	return pieces, ""
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
