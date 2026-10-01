// Package doc renders the declarations of a module as Markdown: one
// document per package, each declaration shown as written with its `///`
// comment, methods grouped under their receiver type.
package doc

import (
	"sort"
	"strings"

	"kigumi/internal/driver"
	"kigumi/internal/syntax"
)

type Options struct {
	// All includes declarations without `pub`.
	All bool
	// Std selects the std packages instead of the module's own.
	Std bool
}

// Module renders every selected package, keyed by package path.
func Module(m *driver.Module, opts Options) map[string]string {
	out := map[string]string{}
	for _, path := range m.Order {
		p := m.Packages[path]
		if p.Std != opts.Std {
			continue
		}
		if text := Package(p, opts); text != "" {
			out[path] = text
		}
	}
	return out
}

// Index lists the rendered packages.
func Index(pages map[string]string) string {
	paths := make([]string, 0, len(pages))
	for p := range pages {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var sb strings.Builder
	sb.WriteString("# Packages\n\n")
	for _, p := range paths {
		sb.WriteString("- [" + p + "](" + p + ".md)\n")
	}
	return sb.String()
}

type entry struct {
	head, doc string
	kind      syntax.NodeKind
	name      string
	recv      string
}

// Package renders one package; "" when it has nothing to show.
func Package(p *driver.Package, opts Options) string {
	var types, ifaces, fns, consts []entry
	methods := map[string][]entry{}
	var owners []string
	for _, t := range p.Files {
		if strings.HasSuffix(t.File.Name, "_test.kg") {
			continue
		}
		for _, d := range t.Children(t.Root) {
			e, ok := declEntry(t, d, opts.All)
			if !ok {
				continue
			}
			switch {
			case e.kind == syntax.FnDecl && e.recv != "":
				if _, seen := methods[e.recv]; !seen {
					owners = append(owners, e.recv)
				}
				methods[e.recv] = append(methods[e.recv], e)
			case e.kind == syntax.FnDecl:
				fns = append(fns, e)
			case e.kind == syntax.TypeDecl:
				types = append(types, e)
			case e.kind == syntax.InterfaceDecl:
				ifaces = append(ifaces, e)
			case e.kind == syntax.ConstDecl:
				consts = append(consts, e)
			}
		}
	}
	if len(types)+len(ifaces)+len(fns)+len(consts)+len(methods) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("# " + p.Path + "\n")
	section(&sb, "Types", types, func(e entry) {
		for _, m := range methods[e.name] {
			write(&sb, "#### ", m)
		}
		delete(methods, e.name)
	})
	section(&sb, "Interfaces", ifaces, nil)
	for _, owner := range owners {
		if ms, ok := methods[owner]; ok {
			sb.WriteString("\n## Methods of " + owner + "\n")
			for _, m := range ms {
				write(&sb, "### ", m)
			}
		}
	}
	section(&sb, "Functions", fns, nil)
	section(&sb, "Constants", consts, nil)
	return sb.String()
}

func section(sb *strings.Builder, title string, es []entry, after func(entry)) {
	if len(es) == 0 {
		return
	}
	sb.WriteString("\n## " + title + "\n")
	for _, e := range es {
		write(sb, "### ", e)
		if after != nil {
			after(e)
		}
	}
}

func write(sb *strings.Builder, prefix string, e entry) {
	name := e.name
	if e.recv != "" {
		name = e.recv + "." + name
	}
	sb.WriteString("\n" + prefix + name + "\n\n```kigumi\n" + e.head + "\n```\n")
	if e.doc != "" {
		sb.WriteString("\n" + e.doc + "\n")
	}
}

// declEntry describes a top-level declaration; ok is false for statements,
// imports and, unless all is set, private declarations.
func declEntry(t *syntax.Tree, d syntax.NodeID, all bool) (entry, bool) {
	kind := t.Kind(d)
	switch kind {
	case syntax.FnDecl, syntax.TypeDecl, syntax.InterfaceDecl, syntax.ConstDecl:
	default:
		return entry{}, false
	}
	if !all && t.Slot(d, "vis") == 0 {
		return entry{}, false
	}
	nameTok := t.Slot(d, "name")
	if nameTok == 0 {
		return entry{}, false
	}
	e := entry{kind: kind, name: t.TokText(nameTok), head: syntax.DeclHead(t, d), doc: syntax.DocText(t, d)}
	if recv := t.Slot(d, "recv"); recv != 0 && kind == syntax.FnDecl {
		sp := t.Span(syntax.NodeID(recv))
		e.recv = ownerName(string(t.File.Src[sp.Start:sp.End]))
	}
	return e, true
}

// ownerName strips type arguments: `Array[T]` and `Map[K, V]` -> `Array`.
func ownerName(recv string) string {
	if i := strings.IndexByte(recv, '['); i >= 0 {
		return recv[:i]
	}
	return recv
}
