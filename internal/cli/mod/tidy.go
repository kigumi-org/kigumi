package mod

import (
	"sort"

	"github.com/spf13/cobra"

	"kigumi/internal/cliutil"
	"kigumi/internal/driver"
	"kigumi/internal/errors"
)

// tidyCmd drops the Requires no package of the module imports, together
// with their Replace and lock records. It never adds a Require: an import of
// an unknown module has no url to fetch from.
func tidyCmd(root *string) *cobra.Command {
	return &cobra.Command{
		Use:   "tidy",
		Short: "Remove Requires that no package imports",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mf, ok, err := driver.ReadModFile(*root)
			if err != nil {
				return errors.WrapErr(err, "read manifest")
			}
			if !ok {
				return &cliutil.UsageError{Err: errors.NewErrf("%s has no %s", *root, driver.ManifestName)}
			}
			m, err := driver.LoadModule(*root, driver.LoadOptions{Test: true, NoDeps: true})
			if err != nil {
				return errors.WrapErr(err, "load module")
			}
			used := m.ImportedModules(mf)
			unused := map[string]bool{}
			for _, r := range mf.Requires {
				if !used[r.Name] {
					unused[r.Name] = true
				}
			}
			if len(unused) == 0 {
				cmd.Println("nothing to remove")
				return nil
			}
			if err := driver.RemoveRequires(*root, unused); err != nil {
				return errors.WrapErr(err, "edit manifest")
			}
			lock, err := driver.ReadLock(*root)
			if err != nil {
				return errors.WrapErr(err, "read lock")
			}
			for name := range unused {
				delete(lock.Entries, name)
				cmd.Printf("removed %s\n", name)
			}
			return driver.WriteLock(*root, lock)
		},
	}
}

func sortedNames(lock driver.Lock) []string {
	names := make([]string, 0, len(lock.Entries))
	for n := range lock.Entries {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
