package driver

import (
	"bytes"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"kigumi/internal/diag"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// ManifestName is the manifest that marks a module root. It is written in
// Kigumi record-literal syntax, one record per statement:
//
//	Module { name: "app" }
//
//	Require {
//	    name: "greet"
//	    version: "v0.1.0"
//	    url: "https://example.com/greet.git"
//	}
//	Replace {
//	    name: "greet"
//	    path: "../greet"
//	}
const ManifestName = "mod.kg"

// LockName records what each Require resolved to (see lockfile.go).
const LockName = "mod.lock.kg"

// IsManifest reports whether a file name is a manifest, a lock file
// (mod.lock.kg or a script's <file>.lock.kg) or the local override; none of
// them is a package file.
func IsManifest(name string) bool {
	return name == ManifestName || name == LocalName || name == FrameworkFileName || strings.HasSuffix(name, ".lock.kg")
}

type ModFile struct {
	Name string
	// Kigumi is the toolchain version the module was written for.
	Kigumi   string
	Requires []Require
	Links    []Link
	// Sources are candidate native sources (a file or a directory of
	// them), relative to the module directory; see Source.
	Sources []NativeSource
	// Headers are C headers to import as generated packages; see
	// HeaderImport in header_import.go.
	Headers []HeaderImport
}

// NativeSource is a `Source { c }` or `Source { asm }` record: a C or assembly
// file, or a directory searched for them. A candidate whose name carries a
// platform suffix (start_amd64.s) is compiled only for that target.
type NativeSource struct {
	Path string `json:"path"`
	Asm  bool   `json:"asm,omitempty"`
}

// Link is a C library (`-l`) or search path (`-L`) the module's extern
// blocks need; source files carry no link attributes.
type Link struct {
	Library, Search string
}

type Require struct {
	Name, Version, URL string
}

// record is one top-level record literal of a manifest-like file.
type record struct {
	Kind   string
	Fields map[string]string
	Span   token.Span
}

var manifestFields = map[string][]string{
	"Module":  {"name", "kigumi"},
	"Require": {"name", "version", "url"},
	"Replace": {"name", "path"},
	"Link":    {"library", "search"},
	"Source":  {"c", "asm"},
	"Header":  {"path", "package"},
}

// ReadModFile parses root/mod.kg; ok is false when there is none.
func ReadModFile(root string) (ModFile, bool, error) {
	src, ok, err := readFileMaybe(filepath.Join(root, ManifestName))
	if err != nil {
		return ModFile{}, false, err
	}
	if !ok {
		return ModFile{}, false, nil
	}
	f := token.NewFile(ManifestName, src)
	mf, ds := ParseModFile(f)
	if len(ds) > 0 {
		return mf, true, fmt.Errorf("%s: %s", filepath.Join(root, ManifestName), diag.RenderAll(f, ds[:1]))
	}
	return mf, true, nil
}

// ParseModFile decodes a manifest; every problem becomes a diagnostic
// positioned in f so editors can show it.
func ParseModFile(f *token.File) (ModFile, []diag.Diagnostic) {
	recs, ds := parseRecords(f, manifestFields)
	mf, more := modFileOf(recs, true)
	return mf, append(ds, more...)
}

// parseRecords reads the top-level record literals of f. allowed maps each
// record kind to its string fields; "name" is required where it is a field.
func parseRecords(f *token.File, allowed map[string][]string) ([]record, []diag.Diagnostic) {
	t := syntax.Parse(f)
	recs, ds := recordsOf(t, t.Children(t.Root), allowed)
	return recs, append(append([]diag.Diagnostic{}, t.Diags...), ds...)
}

// recordsOf decodes the given top-level statements of an already parsed
// tree (a manifest file, or the leading records of a script).
func recordsOf(t *syntax.Tree, stmts []syntax.NodeID, allowed map[string][]string) ([]record, []diag.Diagnostic) {
	f := t.File
	var ds []diag.Diagnostic
	var recs []record
	for _, st := range stmts {
		span := t.Span(st)
		if t.Kind(st) != syntax.ExprStmt || t.Kind(syntax.NodeID(t.Nodes[st].Lhs)) != syntax.RecordLit {
			ds = append(ds, diag.Errorf(diag.Location{Span: span}, "expected a record such as `Require { name: \"x\", version: \"v1.0.0\" }`"))
			continue
		}
		lit := syntax.NodeID(t.Nodes[st].Lhs)
		head := syntax.NodeID(t.Nodes[lit].Lhs)
		kind := t.TokText(t.Nodes[head].Tok)
		fields, ok := allowed[kind]
		if t.Kind(head) != syntax.Ident || !ok {
			ds = append(ds, diag.Errorf(diag.Location{Span: t.Span(head)}, fmt.Sprintf("unknown record %s", kind)))
			continue
		}
		// Closing tokens are not nodes, so widen the span to the `}`.
		if i := bytes.IndexByte(f.Src[span.End:], '}'); i >= 0 {
			span.End += token.Pos(i) + 1
		}
		r := record{Kind: kind, Fields: map[string]string{}, Span: span}
		for _, e := range t.Children(syntax.NodeID(t.Nodes[lit].Rhs)) {
			ds = r.field(t, e, fields, ds)
		}
		if r.Fields["name"] == "" && slices.Contains(fields, "name") {
			ds = append(ds, diag.Errorf(diag.Location{Span: span}, fmt.Sprintf("%s needs a name", kind)))
		}
		recs = append(recs, r)
	}
	return recs, ds
}

func (r *record) field(t *syntax.Tree, e syntax.NodeID, allowed []string, ds []diag.Diagnostic) []diag.Diagnostic {
	if t.Kind(e) != syntax.FieldInit {
		return append(ds, diag.Errorf(diag.Location{Span: t.Span(e)}, "manifest records take `field: \"value\"` entries only"))
	}
	name := t.TokText(t.Nodes[e].Tok)
	if !slices.Contains(allowed, name) {
		return append(ds, diag.Errorf(diag.Location{Span: t.Span(e)}, fmt.Sprintf("%s has no field %s", r.Kind, name)))
	}
	if _, dup := r.Fields[name]; dup {
		return append(ds, diag.Errorf(diag.Location{Span: t.Span(e)}, fmt.Sprintf("field %s given twice", name)))
	}
	val := syntax.NodeID(t.Nodes[e].Lhs)
	s, ok := plainString(t, val)
	if !ok {
		return append(ds, diag.Errorf(diag.Location{Span: t.Span(val)}, fmt.Sprintf("%s must be a plain string literal", name)))
	}
	r.Fields[name] = s
	return ds
}

// plainString decodes a string literal without interpolation.
func plainString(t *syntax.Tree, n syntax.NodeID) (string, bool) {
	if t.Kind(n) != syntax.StringLit {
		return "", false
	}
	out := ""
	for _, p := range t.StringParts(n) {
		if p.Expr != 0 {
			return "", false
		}
		out += syntax.DecodeString(string(t.File.Src[p.Text.Start:p.Text.End]))
	}
	return out, true
}
