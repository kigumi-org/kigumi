// Package lsp implements `kigumi lsp`: the language server on stdio.
package lsp

import (
	"github.com/spf13/cobra"

	"kigumi/internal/cli/shared"
	"kigumi/internal/errors"
	"kigumi/internal/lsp"
)

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lsp",
		Short: "Speak the Language Server Protocol on stdin/stdout",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.WrapErr(lsp.Serve(shared.StdRoot(cmd)), "language server")
		},
	}
}
