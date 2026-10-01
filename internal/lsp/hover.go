package lsp

import (
	"strings"

	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// hoverText shows a declaration as written in its source, followed by its
// doc comment; entities without source fall back to describe.
func hoverText(res *sem.Result, ent sem.EntityID, at sem.TypeID) string {
	code, doc := describe(res, ent, at), ""
	if e := res.Entity(ent); e.File != 0 {
		t := res.Tree(e.File)
		if head := syntax.DeclHead(t, e.Node); head != "" {
			code = head
		}
		doc = syntax.DocText(t, e.Node)
	}
	out := "```kigumi\n" + code + "\n```"
	if doc != "" {
		out += "\n\n" + doc
	}
	return out
}

// describe renders an entity as `kind name: type`.
func describe(res *sem.Result, ent sem.EntityID, at sem.TypeID) string {
	e := res.Entity(ent)
	switch e.Kind {
	case sem.EntFn:
		name := e.Name
		if owner := res.Fn(ent).Owner; owner != 0 {
			name = res.Entity(owner).Name + "." + name
		}
		sig := res.TypeString(res.Fn(ent).Sig)
		i := strings.Index(sig, "fn")
		return sig[:i] + "fn " + name + sig[i+2:]
	case sem.EntType, sem.EntAlias, sem.EntInterface:
		return e.Kind.String() + " " + e.Name
	case sem.EntPackage, sem.EntImport:
		return "package " + e.Name
	case sem.EntVariant:
		return "variant " + res.Entity(e.Parent).Name + "." + e.Name
	case sem.EntField:
		return "field " + e.Name + ": " + res.TypeString(e.Type)
	}
	if at == 0 {
		at = e.Type
	}
	if at == 0 {
		return e.Kind.String() + " " + e.Name
	}
	return e.Kind.String() + " " + e.Name + ": " + res.TypeString(at)
}
