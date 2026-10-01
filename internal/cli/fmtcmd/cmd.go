// Package fmtcmd implements `kigumi fmt`: print, rewrite or check the
// canonical form of source files.
package fmtcmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"kigumi/internal/cliutil"
	"kigumi/internal/driver"
	"kigumi/internal/errors"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fmt [-w|--check] <file>...",
		Short: "Print the canonical form of source files",
		Args:  cobra.MinimumNArgs(1),
	}
	write := cmd.Flags().BoolP("write", "w", false, "rewrite the files in place")
	check := cmd.Flags().Bool("check", false, "exit 1 when a file is not in canonical form")
	cmd.RunE = func(cmd *cobra.Command, paths []string) error {
		if *check && *write {
			return &cliutil.UsageError{Err: errors.NewErr("--check and --write are incompatible")}
		}
		dirty := false
		paths, err := expandDirs(paths)
		if err != nil {
			return err
		}
		for _, path := range paths {
			src, err := driver.Load(path)
			if err != nil {
				return errors.WrapErr(err, "read source")
			}
			out, err := driver.Format(src)
			if err != nil {
				fmt.Fprint(cmd.ErrOrStderr(), err.Error())
				return cliutil.Exit(1)
			}
			switch {
			case *check:
				if out != string(src.Src) {
					dirty = true
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: not formatted\n", path)
				}
			case *write && path != "-":
				if err := driver.WriteFileAtomic(path, []byte(out), 0o644); err != nil {
					return errors.WrapErrf(err, "write %s", path)
				}
			default:
				fmt.Fprint(cmd.OutOrStdout(), out)
			}
		}
		if dirty {
			return cliutil.Exit(1)
		}
		return nil
	}
	return cmd
}

// expandDirs replaces each directory argument with the .kg files under
// it, so `kigumi fmt --check .` covers a module.
func expandDirs(paths []string) ([]string, error) {
	var out []string
	for _, path := range paths {
		st, err := os.Stat(path)
		if err != nil || !st.IsDir() {
			out = append(out, path)
			continue
		}
		err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && p != path && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			if !d.IsDir() && strings.HasSuffix(p, ".kg") && !strings.HasSuffix(p, ".derived.kg") {
				out = append(out, p)
			}
			return nil
		})
		if err != nil {
			return nil, errors.WrapErrf(err, "walk %s", path)
		}
	}
	return out, nil
}
