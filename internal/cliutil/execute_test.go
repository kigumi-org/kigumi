package cliutil_test

import (
	"io"
	"testing"

	"github.com/spf13/cobra"

	"kigumi/internal/cliutil"
	"kigumi/internal/errors"
)

func command(run func() error) *cobra.Command {
	root := &cobra.Command{Use: "t", SilenceUsage: true, SilenceErrors: true, RunE: func(*cobra.Command, []string) error { return run() }}
	root.Flags().Bool("known", false, "")
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root
}

func TestExecuteExitCodes(t *testing.T) {
	cases := []struct {
		name string
		args []string
		run  func() error
		want int
	}{
		{"ok", nil, func() error { return nil }, 0},
		{"failure", nil, func() error { return errors.NewErr("boom") }, 1},
		{"exit", nil, func() error { return cliutil.Exit(3) }, 3},
		{"usage", []string{"--unknown"}, func() error { return nil }, 2},
		{"panic", nil, func() error { panic("boom") }, cliutil.InternalErrorExit},
	}
	for _, c := range cases {
		root := command(c.run)
		root.SetArgs(c.args)
		if got := cliutil.Execute(root, nil); got != c.want {
			t.Errorf("%s: exit %d, want %d", c.name, got, c.want)
		}
	}
}
