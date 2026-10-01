package token

type Kind uint8

const (
	Invalid Kind = iota
	// BOF is a zero-width sentinel at token index 0 so that index 0 can mean
	// "absent" in tree slots that hold token indices.
	BOF
	EOF
	Newline
	Comment
	BlockComment
	DocComment

	Ident
	Int
	Float
	String
	ByteString
	Char
	Lifetime
	Operator

	LParen
	RParen
	LBracket
	RBracket
	LBrace
	RBrace
	Comma
	Dot
	DotDot
	DotDotEq
	Ellipsis
	Colon
	Semicolon
	At
	Dollar
	Question
	QuestionDot
	Arrow
	FatArrow
	Assign

	Plus
	Minus
	Star
	Slash
	Percent
	Amp
	Pipe
	Caret
	Shl
	Shr
	Bang
	Tilde
	EqEq
	NotEq
	Lt
	LtEq
	Gt
	GtEq
	AndAnd
	OrOr
	PipeGt

	PlusAssign
	MinusAssign
	StarAssign
	SlashAssign
	PercentAssign
	AmpAssign
	PipeAssign
	CaretAssign
	ShlAssign
	ShrAssign

	KwFn
	KwLet
	KwConst
	KwMut
	KwType
	KwInterface
	KwPub
	KwImport
	KwIf
	KwElse
	KwMatch
	KwIs
	KwFor
	KwIn
	KwReturn
	KwBreak
	KwContinue
	KwDefer
	KwErrdefer
	KwFail
	KwPure
	KwNoalloc
	KwAsync
	KwAwait
	KwUnsafe
	KwMove
	KwExtern
	KwComptime
	KwTrue
	KwFalse

	kindCount
)

var kindNames = [...]string{
	Invalid: "invalid", BOF: "BOF", EOF: "EOF", Newline: "newline", Comment: "comment",
	BlockComment: "block comment", DocComment: "doc comment",
	Ident: "identifier", Int: "integer", Float: "float", String: "string",
	ByteString: "byte string", Char: "char", Lifetime: "lifetime", Operator: "operator",
	LParen: "(", RParen: ")", LBracket: "[", RBracket: "]", LBrace: "{", RBrace: "}",
	Comma: ",", Dot: ".", DotDot: "..", DotDotEq: "..=", Ellipsis: "...", Colon: ":", Semicolon: ";",
	At: "@", Dollar: "$", Question: "?", QuestionDot: "?.", Arrow: "->", FatArrow: "=>",
	Assign: "=",
	Plus:   "+", Minus: "-", Star: "*", Slash: "/", Percent: "%", Amp: "&", Pipe: "|",
	Caret: "^", Shl: "<<", Shr: ">>", Bang: "!", Tilde: "~", EqEq: "==", NotEq: "!=",
	Lt: "<", LtEq: "<=", Gt: ">", GtEq: ">=", AndAnd: "&&", OrOr: "||", PipeGt: "|>",
	PlusAssign: "+=", MinusAssign: "-=", StarAssign: "*=", SlashAssign: "/=",
	PercentAssign: "%=", AmpAssign: "&=", PipeAssign: "|=", CaretAssign: "^=",
	ShlAssign: "<<=", ShrAssign: ">>=",
	KwFn: "fn", KwLet: "let", KwConst: "const", KwMut: "mut", KwType: "type",
	KwInterface: "interface", KwPub: "pub", KwImport: "import", KwIf: "if", KwElse: "else",
	KwMatch: "match", KwIs: "is", KwFor: "for", KwIn: "in", KwReturn: "return",
	KwBreak: "break", KwContinue: "continue", KwDefer: "defer", KwErrdefer: "errdefer",
	KwFail: "fail", KwPure: "pure", KwNoalloc: "noalloc", KwAsync: "async", KwAwait: "await",
	KwUnsafe: "unsafe", KwMove: "move", KwExtern: "extern", KwComptime: "comptime",
	KwTrue: "true", KwFalse: "false",
}

func (k Kind) String() string {
	if int(k) < len(kindNames) && kindNames[k] != "" {
		return kindNames[k]
	}
	return "kind(" + itoa(int(k)) + ")"
}

func (k Kind) IsKeyword() bool { return k >= KwFn && k < kindCount }

func (k Kind) IsTrivia() bool {
	return k == Comment || k == BlockComment || k == DocComment
}

func (k Kind) IsLiteral() bool {
	switch k {
	case Int, Float, String, ByteString, Char, KwTrue, KwFalse:
		return true
	}
	return false
}

func (k Kind) IsCompoundAssign() bool { return k >= PlusAssign && k <= ShrAssign }

// BinaryOf maps a compound assignment token to its binary operator.
func (k Kind) BinaryOf() Kind {
	switch k {
	case PlusAssign:
		return Plus
	case MinusAssign:
		return Minus
	case StarAssign:
		return Star
	case SlashAssign:
		return Slash
	case PercentAssign:
		return Percent
	case AmpAssign:
		return Amp
	case PipeAssign:
		return Pipe
	case CaretAssign:
		return Caret
	case ShlAssign:
		return Shl
	case ShrAssign:
		return Shr
	}
	return Invalid
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
