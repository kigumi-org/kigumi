// Package parse implements `kigumi parse`: dump the syntax tree.
package parse

import (
	"fmt"

	"github.com/spf13/cobra"

	"kigumi/internal/cliutil"
	"kigumi/internal/driver"
	"kigumi/internal/errors"
)

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "parse <file>",
		Short: "Dump the syntax tree and parse diagnostics of one file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := driver.Load(args[0])
			if err != nil {
				return errors.WrapErr(err, "read source")
			}
			tree, rendered := driver.Parse(src)
			fmt.Fprint(cmd.OutOrStdout(), tree.Dump())
			if rendered != "" {
				fmt.Fprint(cmd.ErrOrStderr(), rendered)
				return cliutil.Exit(1)
			}
			return nil
		},
	}
}
