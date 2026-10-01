// Package explain implements `kigumi explain <code>`: the catalog entry
// behind a diagnostic code, the way `rustc --explain` works.
package explain

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"kigumi/internal/cliutil"
	"kigumi/internal/errors"
	"kigumi/internal/sem"
)

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "explain <code>",
		Short: "Describe a diagnostic code such as E100 (no argument lists them all)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if len(args) == 0 {
				for _, c := range sem.Codes() {
					fmt.Fprintf(out, "%s  %s\n", c.Num, c.Name)
				}
				return nil
			}
			want := strings.ToUpper(args[0])
			for _, c := range sem.Codes() {
				if c.Num != want && c.Name != args[0] {
					continue
				}
				fmt.Fprintf(out, "%s (%s)\n\n  %s\n", c.Num, c.Name, c.Template)
				if c.Help != "" {
					fmt.Fprintf(out, "\n  help: %s\n", c.Help)
				}
				return nil
			}
			return &cliutil.UsageError{Err: errors.NewErrf("unknown diagnostic code %q; `kigumi explain` lists them", args[0])}
		},
	}
}
