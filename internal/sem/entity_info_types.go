package sem

import "kigumi/internal/syntax"

type ImplAssert struct {
	Iface TypeID
	Node  syntax.NodeID
}

type FieldInfo struct {
	Index    int
	Type     TypeID
	Metadata []MetaRef
	// Lifetime is the parameter entity a `&'a T` field's tag names, 0 for a
	// non-reference field (a bare, untagged `&T` field is
	// always an error, so there is no "reference but no lifetime" case).
	Lifetime EntityID
}

// MetaRef is one metadata attribute: the schema function it names and
// its constant arguments.
type MetaRef struct {
	Head EntityID
	Node syntax.NodeID
	Args []Literal
}

type VariantInfo struct {
	Index   int
	Payload []TypeID
	Names   []string
}
