package token

// Pos is a byte offset into the source file.
type Pos uint32

type Span struct {
	Start, End Pos
}

func (s Span) Len() int { return int(s.End - s.Start) }

const (
	// SpaceBefore is set when whitespace separates the token from its predecessor
	// on the same line; the whitespace-call rule depends on it.
	SpaceBefore uint8 = 1 << iota
)

type Token struct {
	Kind  Kind
	Flags uint8
	Span
}

func (t Token) HasSpaceBefore() bool { return t.Flags&SpaceBefore != 0 }

func (t Token) Text(src []byte) string { return string(src[t.Start:t.End]) }
