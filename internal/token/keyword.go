package token

var keywords = map[string]Kind{
	"fn": KwFn, "let": KwLet, "const": KwConst, "mut": KwMut, "type": KwType,
	"interface": KwInterface, "pub": KwPub, "import": KwImport, "if": KwIf,
	"else": KwElse, "match": KwMatch, "is": KwIs, "for": KwFor, "in": KwIn,
	"return": KwReturn, "break": KwBreak, "continue": KwContinue, "defer": KwDefer,
	"errdefer": KwErrdefer, "fail": KwFail, "pure": KwPure, "noalloc": KwNoalloc,
	"async": KwAsync, "await": KwAwait, "unsafe": KwUnsafe, "move": KwMove,
	"extern": KwExtern, "comptime": KwComptime, "true": KwTrue, "false": KwFalse,
}

// Lookup returns the keyword kind for an identifier, or Ident.
func Lookup(ident string) Kind {
	if k, ok := keywords[ident]; ok {
		return k
	}
	return Ident
}

// Contextual keywords stay Ident in the token stream; the parser
// recognizes them by position. They are listed here so tooling can highlight them.
var Contextual = []string{
	"allocator", "resource", "from", "export", "instantiate", "asm",
	"layout", "test", "module", "super", "self",
}
