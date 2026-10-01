package driver

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"kigumi/internal/cheader"
	"kigumi/internal/diag"
	"kigumi/internal/sem"
)

// HeaderImport is a `Header { path, package }` record: a C header, relative
// to the module directory, imported as the package `c/<package>`
type HeaderImport struct {
	Path, Package string
}

// HeaderCacheDir is where imported headers' generated packages are cached,
// keyed by content hash, zig binary fingerprint and target (cheader.Import);
// separate from CacheDir's fetched-module tree since a header import is
// local to this module rather than a dependency someone else published.
func HeaderCacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "kigumi", "cheader"), nil
}

// addHeaders imports each Header record of mf, resolved against dir, as the
// package `c/<package>`. A
// header with unsupported constructs still imports what it can; a single
// warning naming what it skipped attaches to the generated package's file,
// so `kigumi check` and the LSP surface it like any other diagnostic.
func (m *Module) addHeaders(dir string, mf ModFile) error {
	if len(mf.Headers) == 0 {
		return nil
	}
	zigPath, err := exec.LookPath("zig")
	if err != nil {
		return fmt.Errorf("zig not found (needed to import %s's Header records); install zig or remove them", byName(mf))
	}
	cacheDir, err := HeaderCacheDir()
	if err != nil {
		return err
	}
	for _, h := range mf.Headers {
		headerPath := filepath.Join(dir, filepath.FromSlash(h.Path))
		if !withinSandbox([]string{dir}, headerPath) {
			return fmt.Errorf("%s: Header path %q resolves outside the module", mf.Name, h.Path)
		}
		res, err := cheader.Import(cheader.Options{
			ZigPath:    zigPath,
			Target:     m.target.compileTriple(),
			ABI:        cheader.ABIFor(m.target.OS, m.target.Arch),
			HeaderPath: headerPath,
			CacheDir:   cacheDir,
		})
		if err != nil {
			return fmt.Errorf("importing %s (%s): %w", h.Path, h.Package, err)
		}
		if err := m.loadHeaderPackage(h, res); err != nil {
			return err
		}
	}
	return nil
}

// loadHeaderPackage registers one imported header's generated source as the
// package `c/<package>`, loading it like any other package file (real
// spans, so diagnostics, LSP hover and `kigumi doc` all see it normally).
func (m *Module) loadHeaderPackage(h HeaderImport, res cheader.Result) error {
	pkgPath := "c/" + h.Package
	genDir := filepath.Dir(res.CachePath)
	if err := m.loadPackage(genDir, pkgPath, pkgPath, false, false); err != nil {
		return err
	}
	pkg, ok := m.Packages[pkgPath]
	if !ok {
		return nil
	}
	pkg.Header = true
	if len(res.Skipped) == 0 {
		return nil
	}
	f := pkg.Files[0]
	n := len(res.Skipped)
	msg := sem.FillTemplate(sem.HeaderImportSkipped.Template, h.Path, n, sem.Plural(n), strings.Join(res.Skipped, "; "))
	// LineSpan(1) is a valid (zero-length) span even for a header that
	// imported nothing at all, since Generate then writes an empty file.
	warning := diag.Warnf(diag.At(f.File, f.File.LineSpan(1)), msg).WithHelp(sem.HeaderImportSkipped.Help)
	warning.Code = sem.HeaderImportSkipped.Num
	f.Diags = append(f.Diags, warning)
	return nil
}
