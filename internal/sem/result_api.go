package sem

import (
	"slices"
	"strings"

	"kigumi/internal/diag"
	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

func (r *Result) File(t *syntax.Tree) *FileInfo {
	if id, ok := r.fileIndex[t]; ok {
		return &r.Files[id]
	}
	return nil
}

func (r *Result) TypeOf(t *syntax.Tree, n syntax.NodeID) TypeID {
	if f := r.File(t); f != nil && int(n) < len(f.Types) {
		return f.Types[n]
	}
	return 0
}

func (r *Result) EntityOf(t *syntax.Tree, n syntax.NodeID) EntityID {
	if f := r.File(t); f != nil && int(n) < len(f.Uses) {
		return f.Uses[n]
	}
	return 0
}

// Diagnostics returns a file's diagnostics sorted by position, errors first.
func (r *Result) Diagnostics(t *syntax.Tree) []diag.Diagnostic {
	f := r.File(t)
	if f == nil {
		return nil
	}
	return slices.Clone(f.Diags)
}

func (r *Result) HasErrors() bool {
	for i := range r.Files {
		for _, d := range r.Files[i].Diags {
			if d.Severity == diag.Error {
				return true
			}
		}
	}
	return false
}

// Render renders all diagnostics in file order; "" when clean.
func (r *Result) Render() string {
	var sb strings.Builder
	for i := 1; i < len(r.Files); i++ {
		sb.WriteString(diag.RenderAll(r, r.Files[i].Diags))
	}
	return sb.String()
}

// Source resolves a diagnostic location to one of the checked files, so
// notes may point into other files (diag.Sources).
func (r *Result) Source(id token.SourceID) *token.File { return r.sources.File(id) }

// EntityOfName resolves a predeclared (universe) name; tests use it.
func (r *Result) EntityOfName(name string) EntityID { return r.lookupUniverse(name) }
