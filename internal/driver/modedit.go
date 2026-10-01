package driver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kigumi/internal/syntax"
	"kigumi/internal/token"
)

// WriteManifest creates root/mod.kg naming the module and the toolchain
// version it is written for; it refuses to overwrite one.
func WriteManifest(root, name string) error {
	path := filepath.Join(root, ManifestName)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	var sb strings.Builder
	writeRecord(&sb, "Module", "name", name, "kigumi", ToolchainVersion)
	return writeFileAtomic(path, []byte(sb.String()), 0o644)
}

// AppendRequire adds a Require to the end of root/mod.kg, leaving
// everything already there untouched.
func AppendRequire(root string, r Require) error {
	mf, ok, err := ReadModFile(root)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s has no %s; run `kigumi mod init`", root, ManifestName)
	}
	if err := alreadyRequired(mf, r); err != nil {
		return err
	}
	var sb strings.Builder
	sb.WriteString("\n")
	writeRecord(&sb, "Require", "name", r.Name, "version", r.Version, "url", r.URL)
	return appendText(filepath.Join(root, ManifestName), sb.String())
}

// AppendScriptRequire adds a Require to a script's leading records, after
// the ones it has or, for the first, after the shebang and comments
// together with the Kigumi record.
func AppendScriptRequire(path string, r Require) error {
	mf, has, err := ReadScriptManifest(path)
	if err != nil {
		return err
	}
	if err := alreadyRequired(mf, r); err != nil {
		return err
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	t := syntax.Parse(token.NewFile(filepath.Base(path), src))
	var sb strings.Builder
	if !has {
		writeRecord(&sb, "Kigumi", "version", ToolchainVersion)
	}
	writeRecord(&sb, "Require", "name", r.Name, "version", r.Version, "url", r.URL)
	at := len(src)
	if stmts := t.Children(t.Root); len(stmts) > 0 {
		if n := leadingRecords(t); n > 0 {
			recs, _ := recordsOf(t, stmts[:n], scriptFields)
			at = lineEnd(src, int(recs[n-1].Span.End))
		} else {
			at = lineStart(src, int(t.Span(stmts[0]).Start))
			sb.WriteString("\n")
		}
	}
	out := append(append(append([]byte{}, src[:at]...), sb.String()...), src[at:]...)
	return writeFileAtomic(path, out, 0o644)
}

// lineEnd is the offset just past the line containing pos, so the closing
// brace of a record is skipped.
func lineEnd(src []byte, pos int) int {
	for pos < len(src) && src[pos-1] != '\n' {
		pos++
	}
	return pos
}

func lineStart(src []byte, pos int) int {
	for pos > 0 && src[pos-1] != '\n' {
		pos--
	}
	return pos
}

func alreadyRequired(mf ModFile, r Require) error {
	for _, have := range mf.Requires {
		if have.Name == r.Name {
			return fmt.Errorf("%s is already required at %s", r.Name, have.Version)
		}
	}
	return nil
}

// AppendLocalReplace adds a Replace to dir/mod.local.kg, creating it.
func AppendLocalReplace(dir, name, path string) error {
	var sb strings.Builder
	file := filepath.Join(dir, LocalName)
	if _, err := os.Stat(file); err != nil {
		sb.WriteString("// Local overrides for development; never read from a dependency.\n")
	} else {
		sb.WriteString("\n")
	}
	writeRecord(&sb, "Replace", "name", name, "path", path)
	return appendText(file, sb.String())
}

func appendText(path, text string) error {
	src, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(src) > 0 && src[len(src)-1] != '\n' {
		src = append(src, '\n')
	}
	return writeFileAtomic(path, append(src, text...), 0o644)
}

// RemoveRequires deletes the Require records of the named dependencies
// from root/mod.kg, line by line, so comments and the other records keep
// their text.
func RemoveRequires(root string, names map[string]bool) error {
	path := filepath.Join(root, ManifestName)
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	f := token.NewFile(ManifestName, src)
	recs, ds := parseRecords(f, manifestFields)
	if len(ds) > 0 {
		return fmt.Errorf("%s has errors; fix them first", path)
	}
	drop := map[int]bool{}
	for _, r := range recs {
		if r.Kind == "Require" && names[r.Fields["name"]] {
			for line := f.Line(r.Span.Start); line <= f.Line(r.Span.End-1); line++ {
				drop[line] = true
			}
		}
	}
	if len(drop) == 0 {
		return nil
	}
	var sb strings.Builder
	for i, text := range strings.SplitAfter(string(src), "\n") {
		if !drop[i+1] {
			sb.WriteString(text)
		}
	}
	return writeFileAtomic(path, []byte(sb.String()), 0o644)
}

// writeRecord prints a record in the formatter's canonical layout; empty
// values are left out.
func writeRecord(sb *strings.Builder, kind string, kv ...string) {
	var fields []string
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			fields = append(fields, fmt.Sprintf("%s: %q", kv[i], kv[i+1]))
		}
	}
	if len(fields) == 1 {
		fmt.Fprintf(sb, "%s { %s }\n", kind, fields[0])
		return
	}
	fmt.Fprintf(sb, "%s {\n", kind)
	for _, f := range fields {
		fmt.Fprintf(sb, "    %s\n", f)
	}
	sb.WriteString("}\n")
}
