package lsp

import (
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"kigumi/internal/diag"
	"kigumi/internal/token"
)

// toDiagnostic converts a diagnostic of file f; notes become related
// information at their own location, which may lie in another file.
func (s *Server) toDiagnostic(snap *snapshot, f *token.File, d diag.Diagnostic) protocol.Diagnostic {
	msg := d.Msg
	for _, h := range d.Help {
		msg += "\nhelp: " + h
	}
	out := protocol.Diagnostic{Range: spanRange(f, d.Loc.Span), Severity: protocol.DiagnosticSeverity(d.Severity + 1), Source: protocol.NewOptional("kigumi"), Message: protocol.String(msg)}
	if d.Code != "" {
		out.Code = protocol.String(d.Code)
	}
	for _, n := range d.Notes {
		nf := s.fileOf(snap, f, n.Loc)
		loc := protocol.Location{URI: uri.File(s.pathOfFile(snap, nf.Name)), Range: spanRange(nf, n.Loc.Span)}
		out.RelatedInformation = append(out.RelatedInformation, protocol.DiagnosticRelatedInformation{Location: loc, Message: n.Msg})
	}
	return out
}

// fileOf resolves a location against the snapshot, falling back to the
// file the diagnostic was reported in.
func (s *Server) fileOf(snap *snapshot, f *token.File, loc diag.Location) *token.File {
	if snap != nil && snap.res != nil {
		if nf := snap.res.Source(loc.Source); nf != nil {
			return nf
		}
	}
	return f
}

// spanRange converts a byte span to an LSP range (UTF-16 columns).
func spanRange(f *token.File, sp token.Span) protocol.Range {
	return protocol.Range{Start: position(f, sp.Start), End: position(f, sp.End)}
}

func position(f *token.File, pos token.Pos) protocol.Position {
	if int(pos) > len(f.Src) {
		pos = token.Pos(len(f.Src))
	}
	line := f.Line(pos)
	start := f.LineSpan(line).Start
	return protocol.Position{Line: uint32(line - 1), Character: uint32(utf16Len(f.Src[start:pos]))}
}

func utf16Len(b []byte) int {
	n := 0
	for _, r := range string(b) {
		n++
		if r >= 0x10000 {
			n++
		}
	}
	return n
}

// offset converts an LSP position back to a byte offset.
func offset(f *token.File, p protocol.Position) token.Pos {
	if int(p.Line)+1 > f.LineCount() {
		return token.Pos(len(f.Src))
	}
	sp := f.LineSpan(int(p.Line) + 1)
	units := 0
	for i, r := range string(f.Src[sp.Start:sp.End]) {
		if units >= int(p.Character) {
			return sp.Start + token.Pos(i)
		}
		units++
		if r >= 0x10000 {
			units++
		}
	}
	return sp.End
}

func fileOfText(text string) *token.File { return token.NewFile("", []byte(text)) }
