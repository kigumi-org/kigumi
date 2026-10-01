package doc

import (
	"fmt"

	"github.com/spf13/cobra"

	"kigumi/internal/cli/shared"
	"kigumi/internal/cliutil"
	"kigumi/internal/doc"
	"kigumi/internal/driver"
	"kigumi/internal/errors"
)

// SchemaCmd prints the module's declarations with their metadata
// attributes as JSON, the declaration query generators build on.
func SchemaCmd() *cobra.Command {
	var attr string
	var allowErrors bool
	cmd := &cobra.Command{
		Use:   "schema [--attr <pkg.name>] [--allow-errors] <module-root>",
		Short: "List declarations and their metadata attributes as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := shared.Load(cmd, args[0], true, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			res, err := driver.Check(m)
			if err != nil {
				return errors.WrapErr(err, "check module")
			}
			if res.HasErrors() {
				cmd.PrintErr(res.Render())
				if !allowErrors {
					return cliutil.Exit(1)
				}
			}
			fmt.Fprint(cmd.OutOrStdout(), doc.SchemaJSON(doc.Declarations(m, res, attr)))
			return nil
		},
	}
	cmd.Flags().StringVar(&attr, "attr", "", "keep only declarations carrying this attribute")
	cmd.Flags().BoolVar(&allowErrors, "allow-errors", false, "print the declarations even when the module has type errors (generators run before their output exists)")
	return cmd
}
