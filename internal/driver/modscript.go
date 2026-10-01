package driver

import (
	"fmt"
	"os"
	"path/filepath"

	"kigumi/internal/diag"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

var scriptFields = map[string][]string{
	"Kigumi":  {"version"},
	"Require": {"name", "version", "url"},
	"Link":    {"library", "search"},
}

// scriptManifest reads the manifest records a script may carry before its
// first statement and removes them from the tree, so the checker
// never sees them. Inside a module they belong in mod.kg instead.
func (m *Module) scriptManifest(hasMod bool) error {
	for _, p := range m.Packages {
		for _, t := range p.Files {
			n := leadingRecords(t)
			if n == 0 {
				continue
			}
			if hasMod {
				return fmt.Errorf("%s: manifest records go in %s, not in a file of a module", t.File.Name, ManifestName)
			}
			if m.ScriptFile != "" {
				return fmt.Errorf("%s and %s both carry manifest records; only the entry may", m.ScriptFile, t.File.Name)
			}
			stmts := t.Children(t.Root)
			recs, ds := recordsOf(t, stmts[:n], scriptFields)
			mf, more := modFileOf(recs, false)
			ds = append(ds, more...)
			if mf.Kigumi == "" && len(ds) == 0 {
				ds = append(ds, diag.Errorf(diag.Location{Span: t.Span(stmts[0])}, "a script with manifest records needs `Kigumi { version: \"...\" }` first"))
			}
			if len(ds) > 0 {
				return fmt.Errorf("%s", diag.RenderAll(t.File, ds[:1]))
			}
			// The Root list stores its children in Extra; dropping the
			// first n entries leaves the rest of the file untouched.
			t.Nodes[t.Root].Lhs += uint32(n)
			m.Manifest, m.ScriptFile = mf, t.File.Name
		}
	}
	return nil
}

// leadingRecords counts the manifest records at the top of a file.
func leadingRecords(t *syntax.Tree) int {
	n := 0
	for _, st := range t.Children(t.Root) {
		if t.Kind(st) != syntax.ExprStmt {
			break
		}
		lit := syntax.NodeID(t.Nodes[st].Lhs)
		if t.Kind(lit) != syntax.RecordLit {
			break
		}
		head := syntax.NodeID(t.Nodes[lit].Lhs)
		if t.Kind(head) != syntax.Ident {
			break
		}
		if _, ok := scriptFields[t.TokText(t.Nodes[head].Tok)]; !ok {
			break
		}
		n++
	}
	return n
}

// ReadScriptManifest reads the leading records of a script file on disk;
// ok is false when it carries none.
func ReadScriptManifest(path string) (ModFile, bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return ModFile{}, false, err
	}
	t := syntax.Parse(token.NewFile(filepath.Base(path), src))
	n := leadingRecords(t)
	if n == 0 {
		return ModFile{}, false, nil
	}
	recs, ds := recordsOf(t, t.Children(t.Root)[:n], scriptFields)
	mf, more := modFileOf(recs, false)
	if ds = append(ds, more...); len(ds) > 0 {
		return mf, true, fmt.Errorf("%s: %s", path, diag.RenderAll(t.File, ds[:1]))
	}
	return mf, true, nil
}
