package mod

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"kigumi/internal/cliutil"
	"kigumi/internal/driver"
	"kigumi/internal/errors"
)

type manifestJSON struct {
	Name     string                `json:"name"`
	Kigumi   string                `json:"kigumi"`
	Requires []requireJSON         `json:"requires"`
	Links    []driver.Link         `json:"links"`
	Sources  []driver.NativeSource `json:"sources"`
	Local    map[string]string     `json:"local"`
	Lock     map[string]lockJSON   `json:"lock"`
}

type requireJSON struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	URL     string `json:"url,omitempty"`
}

type lockJSON struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	Hash    string `json:"hash"`
}

// jsonCmd prints mod.kg and mod.lock.kg as one JSON document for tools that
// do not parse Kigumi.
func jsonCmd(root *string) *cobra.Command {
	return &cobra.Command{
		Use:   "json",
		Short: "Print the manifest and lock file as JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mf, ok, err := driver.ReadModFile(*root)
			if err != nil {
				return errors.WrapErr(err, "read manifest")
			}
			if !ok {
				return &cliutil.UsageError{Err: errors.NewErrf("%s has no %s", *root, driver.ManifestName)}
			}
			lock, err := driver.ReadLock(*root)
			if err != nil {
				return errors.WrapErr(err, "read lock")
			}
			local, err := driver.ReadLocal(*root)
			if err != nil {
				return errors.WrapErr(err, "read local overrides")
			}
			out := manifestJSON{Name: mf.Name, Kigumi: mf.Kigumi, Requires: []requireJSON{}, Links: mf.Links, Sources: mf.Sources, Local: local.Replaces, Lock: map[string]lockJSON{}}
			for _, r := range mf.Requires {
				out.Requires = append(out.Requires, requireJSON{Name: r.Name, Version: r.Version, URL: r.URL})
			}
			for name, e := range lock.Entries {
				out.Lock[name] = lockJSON{Version: e.Version, Commit: e.Commit, Hash: e.Hash}
			}
			data, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				return err
			}
			cmd.Println(string(data))
			return nil
		},
	}
}
