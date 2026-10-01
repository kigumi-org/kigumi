// Package mod implements `kigumi mod`: create and edit mod.kg, prune unused
// requires, verify the cache against mod.lock.kg and export both as JSON.
package mod

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"kigumi/internal/cliutil"
	"kigumi/internal/driver"
	"kigumi/internal/errors"
)

func Cmd() *cobra.Command {
	root := "."
	cmd := &cobra.Command{
		Use:   "mod",
		Short: "Manage the module manifest (mod.kg) and lock file",
	}
	cmd.PersistentFlags().StringVarP(&root, "chdir", "C", ".", "module root to operate on")
	cmd.AddCommand(initCmd(&root), addCmd(&root), replaceCmd(&root), tidyCmd(&root), verifyCmd(&root), jsonCmd(&root))
	return cmd
}

func initCmd(root *string) *cobra.Command {
	return &cobra.Command{
		Use:   "init [<module path>]",
		Short: "Create mod.kg naming the module (default: the directory name) and the toolchain version",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			} else {
				abs, err := filepath.Abs(*root)
				if err != nil {
					return err
				}
				name = filepath.Base(abs)
			}
			if err := driver.WriteManifest(*root, name); err != nil {
				return &cliutil.UsageError{Err: err}
			}
			cmd.Printf("wrote %s\n", filepath.Join(*root, driver.ManifestName))
			return nil
		},
	}
}

func addCmd(root *string) *cobra.Command {
	var url string
	cmd := &cobra.Command{
		Use:   "add <module path> <version>",
		Short: "Append a Require to mod.kg (run `kigumi get` to fetch it)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := driver.Require{Name: args[0], Version: args[1], URL: url}
			if _, _, ok := driver.RepoOf(r); !ok {
				return &cliutil.UsageError{Err: errors.NewErrf("cannot derive where %s lives; give --url", r.Name)}
			}
			if err := driver.AppendRequire(*root, r); err != nil {
				return &cliutil.UsageError{Err: err}
			}
			cmd.Printf("added %s %s; run `kigumi get` to fetch it\n", r.Name, r.Version)
			return nil
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "clone URL, when it cannot be derived from the module path")
	return cmd
}

func replaceCmd(root *string) *cobra.Command {
	return &cobra.Command{
		Use:   "replace <module path> <directory>",
		Short: "Resolve a dependency from a local directory (written to mod.local.kg)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := driver.AppendLocalReplace(*root, args[0], args[1]); err != nil {
				return errors.WrapErr(err, "edit local overrides")
			}
			cmd.Printf("%s now resolves from %s (%s)\n", args[0], args[1], driver.LocalName)
			return nil
		},
	}
}

func verifyCmd(root *string) *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Check that cached dependencies still match mod.lock.kg",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			lock, err := driver.ReadLock(*root)
			if err != nil {
				return errors.WrapErr(err, "read lock")
			}
			bad := 0
			for _, name := range sortedNames(lock) {
				e := lock.Entries[name]
				dir, err := driver.CacheDir(driver.Require{Name: name, Version: e.Version})
				if err != nil {
					return err
				}
				if _, err := os.Stat(dir); err != nil {
					cmd.Printf("%s %s: not fetched\n", name, e.Version)
					continue
				}
				hash, err := driver.TreeHash(dir)
				if err != nil {
					return errors.WrapErr(err, "hash dependency")
				}
				if hash == e.Hash {
					cmd.Printf("%s %s: ok\n", name, e.Version)
				} else {
					bad++
					cmd.Printf("%s %s: MISMATCH in %s\n", name, e.Version, dir)
				}
			}
			if bad > 0 {
				return cliutil.Exit(1)
			}
			return nil
		},
	}
}
