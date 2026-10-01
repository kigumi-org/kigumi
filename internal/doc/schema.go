package doc

import (
	"encoding/json"
	"strings"

	"kigumi/internal/driver"
	"kigumi/internal/sem"
	"kigumi/internal/syntax"
)

// Declaration is one top-level declaration with its metadata attributes:
// the typed, source-ordered declaration query, exposed to build steps and
// generators as JSON.
type Declaration struct {
	Package    string      `json:"package"`
	Kind       string      `json:"kind"`
	Name       string      `json:"name"`
	Signature  string      `json:"signature"`
	Doc        string      `json:"doc,omitempty"`
	Attributes []Attribute `json:"attributes"`
	Params     []Member    `json:"params,omitempty"`
	Fields     []Member    `json:"fields,omitempty"`
	Variants   []Variant   `json:"variants,omitempty"`
}

// Variant is one constructor of an ADT with its named payload.
type Variant struct {
	Name    string   `json:"name"`
	Payload []Member `json:"payload"`
}

type Attribute struct {
	Name string `json:"name"`
	Args []any  `json:"args"`
}

type Member struct {
	Name       string      `json:"name"`
	Type       string      `json:"type"`
	Attributes []Attribute `json:"attributes"`
}

// Declarations lists the module's own declarations in source order; attr,
// when set, keeps only those carrying that attribute (`web.controller`).
func Declarations(m *driver.Module, res *sem.Result, attr string) []Declaration {
	out := []Declaration{}
	for _, path := range m.Order {
		p := m.Packages[path]
		if p.Std || p.Module != "" {
			continue
		}
		for _, t := range p.Files {
			for _, d := range t.Children(t.Root) {
				if decl, ok := declaration(res, t, d); ok && (attr == "" || hasAttr(decl.Attributes, attr)) {
					decl.Package = path
					out = append(out, decl)
				}
			}
		}
	}
	return out
}

func hasAttr(attrs []Attribute, name string) bool {
	for _, a := range attrs {
		if a.Name == name {
			return true
		}
	}
	return false
}

func declaration(res *sem.Result, t *syntax.Tree, d syntax.NodeID) (Declaration, bool) {
	ent := res.File(t).Defs[d]
	if ent == 0 {
		return Declaration{}, false
	}
	e := res.Entity(ent)
	decl := Declaration{Name: e.Name, Signature: syntax.DeclHead(t, d), Doc: syntax.DocText(t, d), Attributes: []Attribute{}}
	switch e.Kind {
	case sem.EntFn:
		info := res.Fn(ent)
		decl.Kind = "fn"
		if info.Owner != 0 {
			decl.Kind = "method"
			decl.Name = res.Entity(info.Owner).Name + "." + e.Name
		}
		decl.Attributes = attributes(res, t, info.Metadata)
		for _, p := range info.Params {
			pe := res.Entity(p)
			decl.Params = append(decl.Params, Member{Name: pe.Name, Type: res.TypeString(pe.Type), Attributes: attributes(res, t, res.Local(p).Metadata)})
		}
	case sem.EntType:
		info := res.TypeDecl(ent)
		decl.Kind = "type"
		decl.Attributes = attributes(res, t, info.Metadata)
		for _, f := range info.Fields {
			fe := res.Entity(f)
			decl.Fields = append(decl.Fields, Member{Name: fe.Name, Type: res.TypeString(fe.Type), Attributes: attributes(res, t, res.Field(f).Metadata)})
		}
		for _, v := range info.Variants {
			vi := res.Variant(v)
			var payload []Member
			for i, pt := range vi.Payload {
				name := ""
				if i < len(vi.Names) {
					name = vi.Names[i]
				}
				payload = append(payload, Member{Name: name, Type: res.TypeString(pt), Attributes: []Attribute{}})
			}
			decl.Variants = append(decl.Variants, Variant{Name: res.Entity(v).Name, Payload: payload})
		}
	case sem.EntConst:
		decl.Kind = "const"
		decl.Attributes = attributes(res, t, res.Const(ent).Metadata)
	case sem.EntInterface:
		decl.Kind = "interface"
	default:
		return Declaration{}, false
	}
	return decl, true
}

func attributes(res *sem.Result, t *syntax.Tree, refs []sem.MetaRef) []Attribute {
	out := []Attribute{}
	for _, m := range refs {
		var segs []string
		for _, tk := range t.PathToks(syntax.NodeID(t.Nodes[m.Node].Lhs)) {
			segs = append(segs, t.TokText(tk))
		}
		a := Attribute{Name: strings.Join(segs, "."), Args: []any{}}
		for _, lit := range m.Args {
			a.Args = append(a.Args, literalJSON(lit))
		}
		out = append(out, a)
	}
	return out
}

func literalJSON(lit sem.Literal) any {
	switch lit.Kind {
	case sem.LitInt:
		if lit.Int.IsInt64() {
			return lit.Int.Int64()
		}
		return lit.Int.String()
	case sem.LitFloat:
		f, _ := lit.Float.Float64()
		return f
	case sem.LitBool:
		return lit.Bool
	case sem.LitChar:
		return string(lit.Char)
	}
	return lit.Str
}

// SchemaJSON renders the declarations for tools.
func SchemaJSON(decls []Declaration) string {
	out, _ := json.MarshalIndent(decls, "", "  ")
	return string(out) + "\n"
}
