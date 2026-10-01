package driver

import (
	"io"

	"kigumi/internal/diag"
)

// HasParseErrors reports whether any parse diagnostic in scope is an error
// (as opposed to a warning, such as a header import's E994 or the parser's
// own "duplicate modifier"); build.go's compile and cli/shared's Load print
// every diagnostic Diagnostics returns but stop only when this is true.
func (m *Module) HasParseErrors() bool {
	scope := m.buildScope()
	for _, p := range m.Order {
		pkg := m.Packages[p]
		if scope != nil && !pkg.Std && !pkg.Header && !scope[p] {
			continue
		}
		for _, t := range pkg.Files {
			for _, d := range t.Diags {
				if d.Severity == diag.Error {
					return true
				}
			}
		}
	}
	return false
}

// ReportDiagnostics writes m.Diagnostics() to w and reports HasParseErrors,
// but writes only on its first call for m: cli/shared's Load and build.go's
// compile both sit on every CLI path that reaches a build, and would
// otherwise print the same warning twice.
func (m *Module) ReportDiagnostics(w io.Writer) bool {
	if !m.diagsReported {
		m.diagsReported = true
		if d := m.Diagnostics(); d != "" {
			io.WriteString(w, d)
		}
	}
	return m.HasParseErrors()
}
