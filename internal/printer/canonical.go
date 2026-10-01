package printer

import (
	"strings"

	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// Print renders the tree in canonical form. It is only defined
// for trees without diagnostics; callers must check Tree.Diags first.
func Print(t *syntax.Tree) string {
	pr := &printer{t: t, cm: newComments(t)}
	pr.file(t.Root)
	pr.flushComments(uint32(len(t.Toks)))
	return pr.sb.String()
}

type printer struct {
	t      *syntax.Tree
	sb     strings.Builder
	indent int
	// indentBuf is grown (never shrunk) and resliced to the current indent's
	// width, instead of building a fresh strings.Repeat for every line.
	indentBuf []byte
	cm        *comments
	// lineStart is true when nothing has been written on the current line yet.
	lineStart bool
	// chain is set once a statement has broken a method chain onto new lines,
	// so later `.` segments do not indent again.
	chain bool
}

// withIndent runs f and restores the indent (and chain state) afterwards, so
// a multi-line method chain inside f cannot leak its extra indent.
func (p *printer) withIndent(f func()) {
	indent, chain := p.indent, p.chain
	f()
	p.indent, p.chain = indent, chain
}

func (p *printer) write(s string) {
	if p.lineStart && s != "" {
		p.writeIndent()
		p.lineStart = false
	}
	p.sb.WriteString(s)
}

func (p *printer) writeIndent() {
	need := p.indent * 4
	for len(p.indentBuf) < need {
		p.indentBuf = append(p.indentBuf, ' ')
	}
	p.sb.Write(p.indentBuf[:need])
}

func (p *printer) newline() {
	p.sb.WriteString("\n")
	p.lineStart = true
}

func (p *printer) blank() {
	if !strings.HasSuffix(p.sb.String(), "\n\n") && p.sb.Len() > 0 {
		p.newline()
	}
}

func (p *printer) tok(idx uint32) string { return p.t.TokText(idx) }

func (p *printer) node(id syntax.NodeID) syntax.Node { return p.t.Node(id) }

func (p *printer) file(root syntax.NodeID) {
	decls, tight := p.sortImports(p.t.Children(root))
	for i, d := range decls {
		if i > 0 && !tight[d] {
			p.separateDecls(decls[i-1], d)
		}
		p.flushComments(p.firstTok(d))
		p.withIndent(func() { p.decl(d) })
		p.endLine(d)
	}
}

func (p *printer) multiline(id syntax.NodeID) bool {
	sp := p.t.Span(id)
	return p.t.File.Line(sp.Start) != p.t.File.Line(sp.End-1)
}

// firstTok is the first non-trivia token of a node, counting the doc
// comment lines that the node's doc slot points at.
func (p *printer) firstTok(id syntax.NodeID) uint32 {
	if doc := p.t.Slot(id, "doc"); doc != 0 {
		return firstDocTok(p.t, doc)
	}
	sp := p.t.Span(id)
	for i, tk := range p.t.Toks {
		if tk.Start >= sp.Start && tk.Kind != token.Newline && tk.Kind != token.BOF {
			return uint32(i)
		}
	}
	return uint32(len(p.t.Toks))
}

func (p *printer) decl(id syntax.NodeID) {
	switch p.t.Kind(id) {
	case syntax.FnDecl:
		p.fnDecl(id)
	case syntax.TypeDecl:
		p.typeDecl(id)
	case syntax.InterfaceDecl:
		p.interfaceDecl(id)
	case syntax.ConstDecl:
		p.constDecl(id)
	case syntax.ImportDecl:
		p.importDecl(id)
	case syntax.TestDecl:
		n := p.node(id)
		p.write("test " + p.tok(n.Lhs) + " ")
		p.block(syntax.NodeID(n.Rhs))
	case syntax.AbiBlock:
		p.abiBlock(id)
	case syntax.ContractBlock:
		n := p.node(id)
		p.write(modsText(n.Lhs) + " ")
		p.declList(syntax.NodeID(n.Rhs))
	case syntax.InstantiateDecl:
		n := p.node(id)
		p.write("instantiate " + p.tok(n.Tok) + " = ")
		p.typ(syntax.NodeID(n.Rhs))
	default:
		p.stmt(id)
	}
}

func (p *printer) declList(list syntax.NodeID) {
	items := p.t.Children(list)
	p.write("{")
	if len(items) == 0 {
		p.write("}")
		return
	}
	p.newline()
	p.indent++
	for i, d := range items {
		if i > 0 {
			p.separateDecls(items[i-1], d)
		}
		p.flushComments(p.firstTok(d))
		p.withIndent(func() { p.decl(d) })
		p.endLine(d)
	}
	p.indent--
	p.write("}")
}

func modsText(m uint32) string {
	var parts []string
	switch {
	case m&syntax.ModPureVar != 0:
		parts = append(parts, "pure?")
	case m&syntax.ModPure != 0:
		parts = append(parts, "pure")
	}
	for _, e := range []struct {
		bit  uint32
		name string
	}{{syntax.ModNoalloc, "noalloc"}, {syntax.ModAsync, "async"}, {syntax.ModUnsafe, "unsafe"}} {
		if m&e.bit != 0 {
			parts = append(parts, e.name)
		}
	}
	return strings.Join(parts, " ")
}

// firstDocTok walks back from the doc slot token over the `///` lines that
// precede it; the parser records only the last one.
func firstDocTok(t *syntax.Tree, last uint32) uint32 {
	first := last
	for i := int(last) - 1; i > 0; i-- {
		switch t.Toks[i].Kind {
		case token.Newline:
			continue
		case token.DocComment:
			first = uint32(i)
			continue
		}
		break
	}
	return first
}
