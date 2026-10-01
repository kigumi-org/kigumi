package driver

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"kigumi/internal/diag"
)

// modFileOf assembles records into a manifest. module is false for a
// script's leading records, which name no module.
func modFileOf(recs []record, module bool) (ModFile, []diag.Diagnostic) {
	var mf ModFile
	var ds []diag.Diagnostic
	seenModule := false
	for _, r := range recs {
		switch r.Kind {
		case "Module":
			if seenModule {
				ds = append(ds, diag.Errorf(diag.Location{Span: r.Span}, "only one Module record is allowed"))
			}
			seenModule = true
			mf.Name, mf.Kigumi = r.Fields["name"], r.Fields["kigumi"]
			if err := ValidModuleName(mf.Name); err != nil && mf.Name != "" {
				ds = append(ds, diag.Errorf(diag.Location{Span: r.Span}, err.Error()))
			}
			if mf.Kigumi == "" {
				ds = append(ds, diag.Errorf(diag.Location{Span: r.Span}, "Module needs `kigumi`, the toolchain version the module is written for"))
			}
		case "Kigumi":
			mf.Kigumi = r.Fields["version"]
		case "Require":
			if r.Fields["version"] == "" {
				ds = append(ds, diag.Errorf(diag.Location{Span: r.Span}, "Require needs a version"))
			} else if err := ValidVersion(r.Fields["version"]); err != nil {
				ds = append(ds, diag.Errorf(diag.Location{Span: r.Span}, err.Error()))
			}
			if err := ValidModuleName(r.Fields["name"]); err != nil {
				ds = append(ds, diag.Errorf(diag.Location{Span: r.Span}, err.Error()))
			}
			mf.Requires = append(mf.Requires, Require{Name: r.Fields["name"], Version: r.Fields["version"], URL: r.Fields["url"]})
		case "Replace":
			ds = append(ds, diag.Errorf(diag.Location{Span: r.Span}, "Replace belongs in "+LocalName+", which is never published"))
		case "Link":
			ds = append(ds, linkDiags(r)...)
			mf.Links = append(mf.Links, Link{Library: r.Fields["library"], Search: r.Fields["search"]})
		case "Source":
			c, asm := r.Fields["c"], r.Fields["asm"]
			switch {
			case (c == "") == (asm == ""):
				ds = append(ds, diag.Errorf(diag.Location{Span: r.Span}, "Source needs exactly one of `c` or `asm`: a file or a directory relative to the module"))
			case !insideModule(c + asm):
				ds = append(ds, diag.Errorf(diag.Location{Span: r.Span}, "Source paths stay inside the module"))
			}
			mf.Sources = append(mf.Sources, NativeSource{Path: c + asm, Asm: asm != ""})
		case "Header":
			ds = append(ds, headerDiags(r)...)
			mf.Headers = append(mf.Headers, HeaderImport{Path: r.Fields["path"], Package: r.Fields["package"]})
		}
	}
	if mf.Kigumi != "" {
		if atLeast, err := versionAtLeast(ToolchainVersion, mf.Kigumi); err != nil {
			ds = append(ds, diag.Errorf(diag.Location{Span: recs[0].Span}, err.Error()))
		} else if !atLeast {
			ds = append(ds, diag.Errorf(diag.Location{Span: recs[0].Span}, fmt.Sprintf("this module needs kigumi %s; this toolchain is %s", mf.Kigumi, ToolchainVersion)))
		}
	}
	if module && !seenModule && len(recs) > 0 {
		ds = append(ds, diag.Errorf(diag.Location{Span: recs[0].Span}, "the manifest needs a Module record"))
	}
	return mf, ds
}

// linkDiags keeps Link records to what cannot reach outside the module: a
// bare library name and a relative search directory.
func linkDiags(r record) []diag.Diagnostic {
	lib, search := r.Fields["library"], r.Fields["search"]
	switch {
	case lib == "" && search == "":
		return []diag.Diagnostic{diag.Errorf(diag.Location{Span: r.Span}, "Link needs a library or a search path")}
	case strings.ContainsAny(lib, `/\`):
		return []diag.Diagnostic{diag.Errorf(diag.Location{Span: r.Span}, "Link library is a name, not a path")}
	case search != "" && !insideModule(search):
		return []diag.Diagnostic{diag.Errorf(diag.Location{Span: r.Span}, "Link search must be a directory inside the module")}
	}
	return nil
}

var validPackageName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// headerDiags keeps a Header record to a single header file inside the
// module, imported under a plain package-name-shaped identifier.
func headerDiags(r record) []diag.Diagnostic {
	path, pkg := r.Fields["path"], r.Fields["package"]
	switch {
	case path == "" || pkg == "":
		return []diag.Diagnostic{diag.Errorf(diag.Location{Span: r.Span}, "Header needs both `path` and `package`")}
	case !insideModule(path):
		return []diag.Diagnostic{diag.Errorf(diag.Location{Span: r.Span}, "Header path must be a file inside the module")}
	case !validPackageName.MatchString(pkg):
		return []diag.Diagnostic{diag.Errorf(diag.Location{Span: r.Span}, "Header package must look like an identifier (letters, digits, `_`)")}
	}
	return nil
}

// insideModule accepts relative paths that never leave the module.
func insideModule(p string) bool {
	if p == "" || filepath.IsAbs(p) || strings.HasPrefix(p, `\`) {
		return false
	}
	for _, seg := range strings.Split(filepath.ToSlash(p), "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}
