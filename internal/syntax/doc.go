// Package syntax lexes and parses source files into a flat, lossless tree.
//
// Parse always returns a Tree; errors are collected in Tree.Diags and the
// parser resynchronizes at statement boundaries. Nodes reference children and
// tokens by index only, and token index 0 is a zero-width BOF sentinel so that
// 0 can mean "absent" in every slot. Semantic passes keep their results in
// side tables keyed by NodeID and never write into the tree.
//
// Node shapes (see shapes in node_decl.go):
//   - sLeaf: Tok only (Ident, literals, PatWildcard, PatBind flags in Lhs).
//   - sL: Lhs is a child (Paren, Unary, MemberExpr with Tok = name, TryExpr,
//     TypeOptional, TypeResult, Variant with Lhs = field List, AliasBody).
//   - sLR: Lhs and Rhs are children (Binary/IsExpr with Tok = operator,
//     CallExpr/WsCallExpr/BracketExpr with Rhs = argument List, RecordLit,
//     Lambda with Lhs = param List, MatchExpr with Rhs = arm List, TypePath with
//     Lhs = Path and Rhs = type-argument List, Attr, MetaItem, AbiBlock).
//   - sFlagR: Lhs is a flag word, Rhs a child (BorrowExpr, TypeRef, TypePtr,
//     ContractExpr/ContractBlock with modifier bits, VariantField with FlagNamed).
//   - sTokR: Lhs is a token index, Rhs a child (ShellLit, TestDecl, Visibility,
//     InstantiateDecl).
//   - sList: Extra[Lhs:Rhs] are children (File, Block, List, bodies, Metadata,
//     shell nodes).
//   - sRec: Extra[Lhs:Rhs] are named slots; SlotNames gives the names, where a
//     leading '#' marks a flag/int slot and '@' a token index.
//   - sPath: Extra[Lhs:Rhs] are the identifier token indices of a Path.
//
// Ranges `a..b` are Binary nodes whose operator token is DotDot. A ForExpr
// stores the condition or the iterable in its "head" slot; "pattern" is 0 for
// the condition and infinite forms. Receiver type parameters of
// `fn Box[T].value` are GenericParams in the "generics" slot and the receiver
// TypePath refers to them by name.
//
// Helpers: EachChild walks children in slot order, StringParts splits an
// interpolated string, DecodeString/DecodeChar/ParseInt/ParseFloat decode
// literals, VisKind decodes a Visibility node, HasErrors and
// HasTopLevelStatements summarize a file, Dump renders the tree for golden
// tests.
package syntax
