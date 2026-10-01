// Package check implements `kigumi check`: type-check a module.
package check

import (
	"fmt"

	"github.com/spf13/cobra"

	"kigumi/internal/cli/shared"
	"kigumi/internal/cliutil"
	"kigumi/internal/driver"
	"kigumi/internal/errors"
)

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check <module-root|entry.kg>",
		Short: "Type-check a module and print its diagnostics",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := shared.Load(cmd, args[0], false, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			res, err := driver.Check(m)
			if err != nil {
				return errors.WrapErr(err, "check module")
			}
			fmt.Fprint(cmd.ErrOrStderr(), res.Render())
			if res.HasErrors() {
				return cliutil.Exit(1)
			}
			return nil
		},
	}
}
