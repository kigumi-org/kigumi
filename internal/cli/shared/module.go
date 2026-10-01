// Package shared holds the helpers the kigumi subcommands have in common:
// locating the std stubs, loading a module from a root or an entry file,
// and reporting diagnostics.
package shared

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"kigumi/internal/cliutil"
	"kigumi/internal/driver"
	"kigumi/internal/errors"
)

// StdRoot resolves the std stub directory: the --std flag, $KIGUMI_STD,
// a std directory under the working directory, one next to the
// executable, or the copy embedded in the executable.
func StdRoot(cmd *cobra.Command) string {
	if std, _ := cmd.Flags().GetString("std"); std != "" {
		return std
	}
	if env := os.Getenv("KIGUMI_STD"); env != "" {
		return env
	}
	candidates := []string{"std"}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "std"), filepath.Join(filepath.Dir(exe), "..", "std"))
	}
	for _, c := range candidates {
		if st, err := os.Stat(filepath.Join(c, "prelude")); err == nil && st.IsDir() {
			return c
		}
	}
	return embeddedStdRoot()
}

// Load reads the module named by target: a module root, or an entry file.
// Parse diagnostics go to errOut and end the command with exit code 1.
func Load(cmd *cobra.Command, target string, test bool, errOut io.Writer) (*driver.Module, error) {
	root, entry := target, ""
	if strings.HasSuffix(target, ".kg") {
		if _, err := os.Stat(target); err != nil {
			return nil, errors.WrapErrf(err, "load module %s", target)
		}
	} else {
		target = packageEntry(target)
	}
	if strings.HasSuffix(target, ".kg") {
		root = moduleRoot(target)
		rel, err := filepath.Rel(root, target)
		if err != nil {
			return nil, errors.WrapErr(err, "resolve entry file")
		}
		entry = filepath.ToSlash(rel)
	}
	noLocal, _ := cmd.Flags().GetBool("no-local")
	tgt, err := Target(cmd)
	if err != nil {
		return nil, &cliutil.UsageError{Err: err}
	}
	m, err := driver.LoadModule(root, driver.LoadOptions{StdRoot: StdRoot(cmd), Test: test, Entry: entry, NoLocal: noLocal || LocalOff(), Target: tgt})
	if err != nil {
		return nil, errors.WrapErrf(err, "load module %s", root)
	}
	if m.ReportDiagnostics(errOut) {
		return nil, cliutil.Exit(1)
	}
	return m, nil
}

// packageEntry turns a package directory inside a module (`cmd/app`) into
// its entry file, main.kg or its only .kg file; a module root and anything
// else pass through.
func packageEntry(dir string) string {
	if driver.IsModuleRoot(dir) {
		return dir
	}
	if _, ok := driver.FindModuleRoot(dir); !ok {
		return dir
	}
	names, _ := filepath.Glob(filepath.Join(dir, "*.kg"))
	for _, n := range names {
		if filepath.Base(n) == "main.kg" {
			return n
		}
	}
	if len(names) == 1 && !driver.IsManifest(filepath.Base(names[0])) {
		return names[0]
	}
	return dir
}

// moduleRoot is the nearest ancestor of the entry file that is a module
// (mod.kg or cmd/); without one, the entry file's own directory (script
// mode).
func moduleRoot(entry string) string {
	if root, ok := driver.FindModuleRoot(filepath.Dir(entry)); ok {
		return root
	}
	return filepath.Dir(entry)
}

// Target reads the --target and --sys flags.
func Target(cmd *cobra.Command) (driver.Target, error) {
	triple, _ := cmd.Flags().GetString("target")
	sys, _ := cmd.Flags().GetString("sys")
	t, err := driver.ParseTarget(triple, sys)
	if err != nil {
		return t, err
	}
	switch alloc, _ := cmd.Flags().GetString("allocator"); alloc {
	case "", "general":
	case "none":
		t.Allocator = "none"
	default:
		return t, fmt.Errorf("allocator %q: want general or none", alloc)
	}
	return t, nil
}
