package diag

import "kigumi/internal/token"

type Severity uint8

const (
	Error Severity = iota
	Warning
	Note
)

func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warning:
		return "warning"
	}
	return "note"
}

// Location is a span inside one source file, so notes and fixes can point
// at files other than the diagnostic's own.
type Location struct {
	Source token.SourceID
	Span   token.Span
}

// At locates a span in f.
func At(f *token.File, span token.Span) Location {
	return Location{Source: f.ID, Span: span}
}

// Sources resolves locations to files; a *token.File or a
// *token.SourceStore serves.
type Sources interface {
	Source(token.SourceID) *token.File
}

type Diagnostic struct {
	Severity Severity
	Loc      Location
	Msg      string
	// Code is a stable machine-readable id such as "E401".
	Code  string
	Notes []Diagnostic
	// Help lists what the reader can do about it, one suggestion each.
	Help []string
	// Fixes are edits an editor may apply to resolve the diagnostic; Render
	// leaves them out.
	Fixes []Fix
}

// Fix replaces Loc (an empty span inserts).
type Fix struct {
	Title   string
	Loc     Location
	NewText string
}

func Errorf(loc Location, msg string) Diagnostic {
	return Diagnostic{Severity: Error, Loc: loc, Msg: msg}
}

func Warnf(loc Location, msg string) Diagnostic {
	return Diagnostic{Severity: Warning, Loc: loc, Msg: msg}
}

func (d Diagnostic) WithNote(loc Location, msg string) Diagnostic {
	d.Notes = append(d.Notes, Diagnostic{Severity: Note, Loc: loc, Msg: msg})
	return d
}

func (d Diagnostic) WithHelp(msg string) Diagnostic {
	d.Help = append(d.Help, msg)
	return d
}
