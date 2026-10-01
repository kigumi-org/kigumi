package sem

import "kigumi/internal/syntax"

// Typed readers over sRec nodes; slot order follows syntax/node_decl.go.

type fnSlots struct {
	Doc, Name                                     uint32
	Mods                                          uint32
	Attrs, Vis, Recv, Generics, Params, Ret, Body syntax.NodeID
}

func fnDecl(t *syntax.Tree, n syntax.NodeID) fnSlots {
	s := t.Slots(n)
	return fnSlots{Doc: s[0], Attrs: syntax.NodeID(s[1]), Mods: s[2], Vis: syntax.NodeID(s[3]), Recv: syntax.NodeID(s[4]),
		Name: s[5], Generics: syntax.NodeID(s[6]), Params: syntax.NodeID(s[7]), Ret: syntax.NodeID(s[8]), Body: syntax.NodeID(s[9])}
}

type paramSlots struct {
	Attrs, Type syntax.NodeID
	Flags       uint32
}

func param(t *syntax.Tree, n syntax.NodeID) paramSlots {
	s := t.Slots(n)
	return paramSlots{Attrs: syntax.NodeID(s[0]), Flags: s[1], Type: syntax.NodeID(s[2])}
}

type typeDeclSlots struct {
	Doc, Name                          uint32
	Attrs, Vis, Generics, Layout, Body syntax.NodeID
}

func typeDecl(t *syntax.Tree, n syntax.NodeID) typeDeclSlots {
	s := t.Slots(n)
	return typeDeclSlots{Doc: s[0], Attrs: syntax.NodeID(s[1]), Vis: syntax.NodeID(s[2]), Name: s[3],
		Generics: syntax.NodeID(s[4]), Layout: syntax.NodeID(s[5]), Body: syntax.NodeID(s[6])}
}

type fieldSlots struct {
	Vis, Type, Metadata syntax.NodeID
	Flags               uint32
}

func field(t *syntax.Tree, n syntax.NodeID) fieldSlots {
	s := t.Slots(n)
	return fieldSlots{Vis: syntax.NodeID(s[0]), Flags: s[1], Type: syntax.NodeID(s[2]), Metadata: syntax.NodeID(s[3])}
}

type ifaceSlots struct {
	Doc, Name                     uint32
	Attrs, Vis, Generics, Members syntax.NodeID
}

func ifaceDecl(t *syntax.Tree, n syntax.NodeID) ifaceSlots {
	s := t.Slots(n)
	return ifaceSlots{Doc: s[0], Attrs: syntax.NodeID(s[1]), Vis: syntax.NodeID(s[2]), Name: s[3],
		Generics: syntax.NodeID(s[4]), Members: syntax.NodeID(s[5])}
}

type constSlots struct {
	Doc, Name               uint32
	Attrs, Vis, Type, Value syntax.NodeID
}

func constDecl(t *syntax.Tree, n syntax.NodeID) constSlots {
	s := t.Slots(n)
	return constSlots{Doc: s[0], Attrs: syntax.NodeID(s[1]), Vis: syntax.NodeID(s[2]), Name: s[3],
		Type: syntax.NodeID(s[4]), Value: syntax.NodeID(s[5])}
}

type importSlots struct {
	Vis, Binding, Path syntax.NodeID
}

func importDecl(t *syntax.Tree, n syntax.NodeID) importSlots {
	s := t.Slots(n)
	return importSlots{Vis: syntax.NodeID(s[0]), Binding: syntax.NodeID(s[1]), Path: syntax.NodeID(s[2])}
}

type letSlots struct {
	Flags                     uint32
	Pattern, Type, Init, Else syntax.NodeID
}

func letStmt(t *syntax.Tree, n syntax.NodeID) letSlots {
	s := t.Slots(n)
	return letSlots{Flags: s[0], Pattern: syntax.NodeID(s[1]), Type: syntax.NodeID(s[2]), Init: syntax.NodeID(s[3]), Else: syntax.NodeID(s[4])}
}

type ifSlots struct {
	Cond, Then, Else syntax.NodeID
}

func ifExpr(t *syntax.Tree, n syntax.NodeID) ifSlots {
	s := t.Slots(n)
	return ifSlots{Cond: syntax.NodeID(s[0]), Then: syntax.NodeID(s[1]), Else: syntax.NodeID(s[2])}
}

type ifLetSlots struct {
	Flags                            uint32
	Pattern, Init, Guard, Then, Else syntax.NodeID
}

func ifLet(t *syntax.Tree, n syntax.NodeID) ifLetSlots {
	s := t.Slots(n)
	return ifLetSlots{Flags: s[0], Pattern: syntax.NodeID(s[1]), Init: syntax.NodeID(s[2]), Guard: syntax.NodeID(s[3]),
		Then: syntax.NodeID(s[4]), Else: syntax.NodeID(s[5])}
}

type armSlots struct {
	Pattern, Guard, Body syntax.NodeID
}

func matchArm(t *syntax.Tree, n syntax.NodeID) armSlots {
	s := t.Slots(n)
	return armSlots{Pattern: syntax.NodeID(s[0]), Guard: syntax.NodeID(s[1]), Body: syntax.NodeID(s[2])}
}

type forSlots struct {
	Pattern, Head, Body syntax.NodeID
}

func forExpr(t *syntax.Tree, n syntax.NodeID) forSlots {
	s := t.Slots(n)
	return forSlots{Pattern: syntax.NodeID(s[0]), Head: syntax.NodeID(s[1]), Body: syntax.NodeID(s[2])}
}

type fnTypeSlots struct {
	Mods, Abi, Flags uint32
	Params, Ret      syntax.NodeID
}

func fnType(t *syntax.Tree, n syntax.NodeID) fnTypeSlots {
	s := t.Slots(n)
	return fnTypeSlots{Mods: s[0], Abi: s[1], Params: syntax.NodeID(s[2]), Flags: s[3], Ret: syntax.NodeID(s[4])}
}

func pathText(t *syntax.Tree, n syntax.NodeID, sep string) string {
	out := ""
	for i, tk := range t.PathToks(n) {
		if i > 0 {
			out += sep
		}
		out += t.TokText(tk)
	}
	return out
}
